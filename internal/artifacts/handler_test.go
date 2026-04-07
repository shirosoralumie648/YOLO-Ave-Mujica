package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/audit"
	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/observability"
	"yolo-ave-mujica/internal/server"
)

type resolveRepoStub struct {
	byDataset map[string]Artifact
	resolved  Artifact
}

type projectScopeResolverStub struct {
	projectID int64
	err       error
}

func (r projectScopeResolverStub) ResolveProjectID(_ context.Context, _, _ int64) (int64, error) {
	return r.projectID, r.err
}

func (r resolveRepoStub) Create(_ context.Context, a Artifact) (Artifact, error) {
	return a, nil
}

func (r resolveRepoStub) Get(_ context.Context, id int64) (Artifact, bool, error) {
	if r.resolved.ID == id {
		return r.resolved, true, nil
	}
	return Artifact{}, false, nil
}

func (r resolveRepoStub) FindReadyByFormatVersion(_ context.Context, format, version string) (Artifact, bool, error) {
	if r.resolved.Format == format && r.resolved.Version == version {
		return r.resolved, true, nil
	}
	return Artifact{}, false, nil
}

func (r resolveRepoStub) FindReadyByDatasetFormatVersion(_ context.Context, dataset, format, version string) (Artifact, bool, error) {
	if dataset != "" {
		if artifact, ok := r.byDataset[dataset]; ok && artifact.Format == format && artifact.Version == version {
			return artifact, true, nil
		}
	}
	if r.resolved.Format == format && r.resolved.Version == version {
		return r.resolved, true, nil
	}
	return Artifact{}, false, nil
}

func (r resolveRepoStub) LinkJob(_ context.Context, _ int64, _ int64) (Artifact, error) {
	return r.resolved, nil
}

func (r resolveRepoStub) UpdateStatus(_ context.Context, _ int64, _ string, _ string) (Artifact, error) {
	return r.resolved, nil
}

func (r resolveRepoStub) UpdateBuildResult(_ context.Context, _ int64, _ BuildResult) (Artifact, error) {
	return r.resolved, nil
}

func (r resolveRepoStub) MarkStaleBuildsFailed(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func TestCreatePackageReturnsJobID(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:   h.CreatePackage,
			GetArtifact:     h.GetArtifact,
			PresignArtifact: h.PresignArtifact,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo"}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "job_id") {
		t.Fatalf("expected async package response, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePackageWritesAuditEvent(t *testing.T) {
	svc := NewService()
	recorder := audit.NewRecorder()
	h := NewHandlerWithJobsAndAudit(svc, nil, recorder)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage: h.CreatePackage,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v-audit"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected async package response, got %d %s", rec.Code, rec.Body.String())
	}

	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %+v", events)
	}
	if events[0].Action != "artifact.package.request" || events[0].ResourceType != "artifact" || events[0].ResourceID != "1" {
		t.Fatalf("unexpected audit event: %+v", events[0])
	}
}

func TestCreatePackageUsesResolvedProjectScopeForAuthorization(t *testing.T) {
	svc := NewService().WithProjectScopeResolver(projectScopeResolverStub{projectID: 2})
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		MutationMiddleware: auth.IdentityMiddleware([]int64{1}),
		Artifacts: server.ArtifactRoutes{
			CreatePackage: h.CreatePackage,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"project_id":1,"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v-resolved-authz"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPresignArtifactRejectsProjectOutsideCallerScope(t *testing.T) {
	repo := NewInMemoryRepository()
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID:    2,
		DatasetID:    1,
		SnapshotID:   2,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		Status:       StatusReady,
		URI:          "artifact://v1/package.yolo.tar.gz",
		ManifestURI:  "artifact://v1/manifest.json",
		Checksum:     "sha256:abc123",
		Size:         123,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	svc := NewServiceWithRepository(repo)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			PresignArtifact: h.PresignArtifact,
		},
		MutationMiddleware: auth.IdentityMiddleware([]int64{1}),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/presign", strings.NewReader(`{"ttl_seconds":60}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExportSnapshotWritesAuditEvent(t *testing.T) {
	svc := NewService()
	recorder := audit.NewRecorder()
	h := NewHandlerWithJobsAndAudit(svc, nil, recorder)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"dataset_id":1,"format":"yolo","version":"v-audit"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected async export response, got %d %s", rec.Code, rec.Body.String())
	}

	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %+v", events)
	}
	if events[0].Action != "artifact.package.request" || events[0].ResourceType != "artifact" || events[0].ResourceID != "1" {
		t.Fatalf("unexpected audit event: %+v", events[0])
	}
}

func TestExportSnapshotUsesResolvedProjectScopeForAuthorization(t *testing.T) {
	svc := NewService().WithProjectScopeResolver(projectScopeResolverStub{projectID: 2})
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		MutationMiddleware: auth.IdentityMiddleware([]int64{1}),
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"project_id":1,"dataset_id":1,"format":"yolo","version":"v-resolved-authz"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatePackageReturnsArtifactIDAndRejectsPresignUntilReady(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:   h.CreatePackage,
			GetArtifact:     h.GetArtifact,
			PresignArtifact: h.PresignArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		JobID      int64 `json:"job_id"`
		ArtifactID int64 `json:"artifact_id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.ArtifactID == 0 {
		t.Fatalf("expected artifact_id in create response, got %+v", createResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1", nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get artifact failed: %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"status":"pending"`) {
		t.Fatalf("expected pending artifact after queueing, got body=%s", getRec.Body.String())
	}

	presignReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/1/presign", strings.NewReader(`{"ttl_seconds":60}`))
	presignRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(presignRec, presignReq)
	if presignRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for pending artifact presign, got %d body=%s", presignRec.Code, presignRec.Body.String())
	}
	if !strings.Contains(presignRec.Body.String(), "pending") {
		t.Fatalf("expected pending artifact status in presign error, got %s", presignRec.Body.String())
	}
}

func TestGetArtifactReturnsNotFoundOutsideCallerScope(t *testing.T) {
	repo := NewInMemoryRepository()
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID:    2,
		DatasetID:    1,
		SnapshotID:   2,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		Status:       StatusReady,
		URI:          "artifact://v1/package.yolo.tar.gz",
		ManifestURI:  "artifact://v1/manifest.json",
		Checksum:     "sha256:abc123",
		Size:         123,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	h := NewHandler(NewServiceWithRepository(repo))
	srv := server.NewHTTPServerWithModules(server.Modules{
		HTTPMiddleware: auth.IdentityMiddleware([]int64{1}),
		Artifacts: server.ArtifactRoutes{
			GetArtifact: h.GetArtifact,
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+strconv.FormatInt(artifact.ID, 10), nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDownloadArtifactRejectsPendingArtifactWithConflict(t *testing.T) {
	svc := NewServiceWithDependencies(NewInMemoryRepository(), nil, nil, newArtifactStorageStub())
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    h.CreatePackage,
			DownloadArtifact: h.DownloadArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1/download", nil)
	downloadRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for pending artifact download, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if !strings.Contains(downloadRec.Body.String(), "pending") {
		t.Fatalf("expected pending artifact status in download error, got %s", downloadRec.Body.String())
	}
}

func TestDownloadArtifactReturnsNotFoundOutsideCallerScope(t *testing.T) {
	store := newArtifactStorageStub()
	store.uploads["artifact://v1/package.yolo.tar.gz"] = []byte("archive")
	repo := NewInMemoryRepository()
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID:    2,
		DatasetID:    1,
		SnapshotID:   2,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		Status:       StatusReady,
		URI:          "artifact://v1/package.yolo.tar.gz",
		ManifestURI:  "artifact://v1/manifest.json",
		Checksum:     "sha256:abc123",
		Size:         int64(len("archive")),
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	h := NewHandler(NewServiceWithDependencies(repo, nil, nil, store))
	srv := server.NewHTTPServerWithModules(server.Modules{
		HTTPMiddleware: auth.IdentityMiddleware([]int64{1}),
		Artifacts: server.ArtifactRoutes{
			DownloadArtifact: h.DownloadArtifact,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/download", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResolveArtifactByFormatAndVersion(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(repo)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:   h.CreatePackage,
			GetArtifact:     h.GetArtifact,
			PresignArtifact: h.PresignArtifact,
			ResolveArtifact: h.ResolveArtifact,
		},
	})

	created, err := repo.Create(context.Background(), Artifact{
		ProjectID:    1,
		DatasetID:    1,
		SnapshotID:   2,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		Status:       StatusPending,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, err := repo.UpdateBuildResult(context.Background(), created.ID, BuildResult{
		Status:      StatusReady,
		URI:         "artifact://v1/package.yolo.tar.gz",
		ManifestURI: "artifact://v1/manifest.json",
		Checksum:    "sha256:abc123",
		Size:        123,
	}); err != nil {
		t.Fatalf("mark artifact ready: %v", err)
	}

	resolveReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/resolve?format=yolo&version=v1", nil)
	resolveRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve artifact failed: %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	var artifact Artifact
	if err := json.NewDecoder(resolveRec.Body).Decode(&artifact); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if artifact.Format != "yolo" || artifact.Version != "v1" {
		t.Fatalf("unexpected resolved artifact: %+v", artifact)
	}
}

func TestResolveArtifactHonorsDatasetQuery(t *testing.T) {
	h := NewHandler(NewServiceWithRepository(resolveRepoStub{
		byDataset: map[string]Artifact{
			"1": {
				ID:           1,
				ProjectID:    1,
				DatasetID:    1,
				ArtifactType: "package",
				Format:       "yolo",
				Version:      "v1",
				Status:       "ready",
			},
		},
		resolved: Artifact{
			ID:           2,
			ProjectID:    1,
			DatasetID:    2,
			ArtifactType: "package",
			Format:       "yolo",
			Version:      "v1",
			Status:       "ready",
		},
	}))
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ResolveArtifact: h.ResolveArtifact,
		},
	})

	resolveReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/resolve?dataset=1&format=yolo&version=v1", nil)
	resolveRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve artifact failed: %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	var artifact Artifact
	if err := json.NewDecoder(resolveRec.Body).Decode(&artifact); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if artifact.DatasetID != 1 {
		t.Fatalf("expected dataset-specific resolve to return dataset 1 artifact, got %+v", artifact)
	}
}

func TestResolveArtifactReturnsNotFoundOutsideCallerScope(t *testing.T) {
	repo := NewInMemoryRepository()
	if _, err := repo.Create(context.Background(), Artifact{
		ProjectID:    2,
		DatasetID:    1,
		SnapshotID:   2,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		Status:       StatusReady,
		URI:          "artifact://v1/package.yolo.tar.gz",
		ManifestURI:  "artifact://v1/manifest.json",
		Checksum:     "sha256:abc123",
		Size:         123,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	h := NewHandler(NewServiceWithRepository(repo))
	req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/resolve?format=yolo&version=v1", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.NewIdentity("reviewer-1", []int64{1})))
	rec := httptest.NewRecorder()

	h.ResolveArtifact(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExportSnapshotQueuesPackageJob(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"dataset_id":1,"format":"yolo","version":"v2"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"job_id"`) || !strings.Contains(rec.Body.String(), `"artifact_id"`) {
		t.Fatalf("expected job and artifact ids, got %s", rec.Body.String())
	}
}

func TestExportSnapshotAcceptsCOCOFormat(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"dataset_id":1,"format":"coco","version":"v2"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"artifact_id"`) {
		t.Fatalf("expected artifact id for coco export, got %s", rec.Body.String())
	}
}

func TestExportSnapshotRejectsUnsupportedFormat(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"dataset_id":1,"format":"pascal_voc","version":"v2"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got %s", rec.Body.String())
	}
}

func TestExportSnapshotQueuesArtifactPackageJobWhenJobsConfigured(t *testing.T) {
	artifactSvc := NewService()
	jobsRepo := jobs.NewInMemoryRepository()
	pub := jobs.NewInMemoryPublisher()
	jobsSvc := jobs.NewServiceWithPublisher(jobsRepo, pub)
	h := NewHandlerWithJobs(artifactSvc, jobsSvc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"dataset_id":1,"format":"yolo","version":"v2"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		JobID      int64 `json:"job_id"`
		ArtifactID int64 `json:"artifact_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	if resp.JobID == 0 || resp.ArtifactID == 0 {
		t.Fatalf("expected job_id and artifact_id, got %+v", resp)
	}

	job, ok := jobsSvc.GetJob(resp.JobID)
	if !ok {
		t.Fatalf("expected persisted artifact-package job %d", resp.JobID)
	}
	if job.JobType != "artifact-package" {
		t.Fatalf("expected artifact-package job, got %+v", job)
	}
	gotArtifactID, ok := job.Payload["artifact_id"].(int64)
	if !ok || gotArtifactID != resp.ArtifactID {
		t.Fatalf("expected artifact_id %d in payload, got %+v", resp.ArtifactID, job.Payload)
	}
	if pub.LastLane() != "jobs:cpu" {
		t.Fatalf("expected jobs:cpu lane, got %s", pub.LastLane())
	}
}

func TestGetArtifactIncludesCreatedByJobIDWhenQueuedViaJobs(t *testing.T) {
	artifactRepo := NewInMemoryRepository()
	artifactSvc := NewServiceWithRepository(artifactRepo)
	jobsRepo := jobs.NewInMemoryRepository()
	pub := jobs.NewInMemoryPublisher()
	jobsSvc := jobs.NewServiceWithPublisher(jobsRepo, pub)
	h := NewHandlerWithJobs(artifactSvc, jobsSvc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			ExportSnapshot: h.ExportSnapshot,
			GetArtifact:    h.GetArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/snapshots/2/export", strings.NewReader(`{"dataset_id":1,"format":"yolo","version":"v2"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		JobID      int64 `json:"job_id"`
		ArtifactID int64 `json:"artifact_id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+strconv.FormatInt(createResp.ArtifactID, 10), nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var artifact struct {
		ID             int64  `json:"id"`
		CreatedByJobID *int64 `json:"created_by_job_id"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&artifact); err != nil {
		t.Fatalf("decode artifact response: %v", err)
	}
	if artifact.CreatedByJobID == nil || *artifact.CreatedByJobID != createResp.JobID {
		t.Fatalf("expected created_by_job_id=%d, got %+v", createResp.JobID, artifact)
	}
}

func TestGetArtifactIncludesBundleEntries(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage: h.CreatePackage,
			GetArtifact:   h.GetArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v1"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1", nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get artifact failed: %d body=%s", getRec.Code, getRec.Body.String())
	}

	var artifact struct {
		ID      int64 `json:"id"`
		Entries []struct {
			Path     string `json:"path"`
			Body     []byte `json:"body"`
			Checksum string `json:"checksum"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&artifact); err != nil {
		t.Fatalf("decode artifact response: %v", err)
	}
	if artifact.ID == 0 {
		t.Fatalf("expected artifact id, got %+v", artifact)
	}
	if len(artifact.Entries) == 0 {
		t.Fatalf("expected bundle entries in get artifact response, got %+v", artifact)
	}
}

func TestCompleteArtifactMarksReadyAndGetArtifactReturnsStorageMetadata(t *testing.T) {
	store := newArtifactStorageStub()
	svc := NewServiceWithRepositoryAndStorage(NewInMemoryRepository(), store.upload, store.presign)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    h.CreatePackage,
			GetArtifact:      h.GetArtifact,
			PresignArtifact:  h.PresignArtifact,
			CompleteArtifact: h.CompleteArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v1"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(`{
		"entries":[
			{
				"path":"labels/0001.txt",
				"body":"MCAwLjUgMC41IDAuMiAwLjIK",
				"checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"
			}
		]
	}`))
	completeRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete artifact failed: %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1", nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get artifact failed: %d body=%s", getRec.Code, getRec.Body.String())
	}

	var artifact Artifact
	if err := json.NewDecoder(getRec.Body).Decode(&artifact); err != nil {
		t.Fatalf("decode artifact response: %v", err)
	}
	if artifact.Status != "ready" || artifact.URI == "" || artifact.ManifestURI == "" {
		t.Fatalf("expected ready storage metadata, got %+v", artifact)
	}
	if len(artifact.Entries) != 0 {
		t.Fatalf("expected storage-backed artifact to omit inline entries, got %+v", artifact)
	}

	presignReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/1/presign", strings.NewReader(`{"ttl_seconds":60}`))
	presignRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(presignRec, presignReq)
	if presignRec.Code != http.StatusOK || !strings.Contains(presignRec.Body.String(), "http://download.local/") {
		t.Fatalf("expected ready artifact presign to use storage-backed URL, got %d %s", presignRec.Code, presignRec.Body.String())
	}

	if len(store.uploads) != 2 {
		t.Fatalf("expected package and manifest uploads, got %d", len(store.uploads))
	}
}

func TestCompleteArtifactRecordsReadyMetric(t *testing.T) {
	store := newArtifactStorageStub()
	metrics := observability.NewMetrics()
	svc := NewServiceWithRepositoryAndStorage(NewInMemoryRepository(), store.upload, store.presign)
	svc = svc.WithMetrics(metrics)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    h.CreatePackage,
			CompleteArtifact: h.CompleteArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v-metric"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(`{
		"entries":[
			{
				"path":"labels/0001.txt",
				"body":"MCAwLjUgMC41IDAuMiAwLjIK",
				"checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"
			}
		]
	}`))
	completeRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete artifact failed: %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `yolo_artifact_build_outcomes_total{status="ready"} 1`) {
		t.Fatalf("expected artifact ready metric, got:\n%s", metricsRec.Body.String())
	}
}

type artifactStorageStub struct {
	uploads     map[string][]byte
	uploadCalls int
	storeCalls  int
}

func newArtifactStorageStub() *artifactStorageStub {
	return &artifactStorageStub{uploads: make(map[string][]byte)}
}

func (s *artifactStorageStub) upload(uri string, body []byte, _ string) (int64, error) {
	s.uploadCalls++
	s.uploads[uri] = append([]byte(nil), body...)
	return int64(len(body)), nil
}

func (s *artifactStorageStub) presign(uri string, ttlSeconds int) (string, error) {
	return "http://download.local/" + strings.TrimPrefix(uri, "s3://"), nil
}

func (s *artifactStorageStub) StoreBuild(_ context.Context, _ StoreRequest) (StoredArtifact, error) {
	s.storeCalls++
	return StoredArtifact{
		ArchivePath:  "/tmp/package.yolo.tar.gz",
		ManifestPath: "/tmp/manifest.json",
		ArchiveURI:   "artifact://v1/package.yolo.tar.gz",
		ManifestURI:  "artifact://v1/manifest.json",
		ArchiveSize:  int64(len("archive")),
	}, nil
}

func (s *artifactStorageStub) OpenArchive(_ context.Context, _ string) (ReadSeekCloser, int64, error) {
	body := []byte("archive")
	return &readSeekCloser{Reader: bytes.NewReader(body)}, int64(len(body)), nil
}

type readSeekCloser struct {
	*bytes.Reader
}

func (r *readSeekCloser) Close() error {
	return nil
}

func TestCompleteArtifactIsIdempotentWhenAlreadyReady(t *testing.T) {
	store := newArtifactStorageStub()
	svc := NewServiceWithRepositoryAndStorage(NewInMemoryRepository(), store.upload, store.presign)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    h.CreatePackage,
			CompleteArtifact: h.CompleteArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v1"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	body := `{
		"entries":[
			{
				"path":"labels/0001.txt",
				"body":"MCAwLjUgMC41IDAuMiAwLjIK",
				"checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"
			}
		]
	}`
	completeReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(body))
	completeRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("first complete artifact failed: %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(body))
	retryRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent complete callback, got %d body=%s", retryRec.Code, retryRec.Body.String())
	}
	if store.uploadCalls != 2 {
		t.Fatalf("expected package and manifest uploads only once, got %d uploads", store.uploadCalls)
	}
}

func TestCompleteArtifactReturnsConflictForDifferentReplay(t *testing.T) {
	store := newArtifactStorageStub()
	svc := NewServiceWithRepositoryAndStorage(NewInMemoryRepository(), store.upload, store.presign)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    h.CreatePackage,
			CompleteArtifact: h.CompleteArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v1"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	firstBody := `{
		"entries":[
			{
				"path":"labels/0001.txt",
				"body":"MCAwLjUgMC41IDAuMiAwLjIK",
				"checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"
			}
		]
	}`
	firstReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(firstBody))
	firstRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first complete artifact failed: %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	conflictBody := `{
		"entries":[
			{
				"path":"labels/0002.txt",
				"body":"MCAwLjQgMC40IDAuMSAwLjEK",
				"checksum":"c028d7aa15e851b0eefb31638a1856498a237faf1829050832d3b9b19f9ab75f"
			}
		]
	}`
	conflictReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(conflictBody))
	conflictRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(conflictRec, conflictReq)

	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("expected conflicting replay to return 409, got %d body=%s", conflictRec.Code, conflictRec.Body.String())
	}
	if !strings.Contains(strings.ToLower(conflictRec.Body.String()), "different bundle entries") {
		t.Fatalf("expected conflict detail, got %s", conflictRec.Body.String())
	}
	if store.uploadCalls != 2 {
		t.Fatalf("expected conflicting replay to avoid extra uploads, got %d uploads", store.uploadCalls)
	}
}

func TestCompleteArtifactUsesConfiguredStorageBackendWithoutUploadHooks(t *testing.T) {
	store := newArtifactStorageStub()
	svc := NewServiceWithDependencies(NewInMemoryRepository(), nil, nil, store)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    h.CreatePackage,
			CompleteArtifact: h.CompleteArtifact,
			GetArtifact:      h.GetArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v1"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/internal/artifacts/1/complete", strings.NewReader(`{
		"entries":[
			{
				"path":"labels/0001.txt",
				"body":"MCAwLjUgMC41IDAuMiAwLjIK",
				"checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"
			}
		]
	}`))
	completeRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("expected storage-backed complete artifact to succeed, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1", nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get artifact failed: %d body=%s", getRec.Code, getRec.Body.String())
	}

	var artifact Artifact
	if err := json.NewDecoder(getRec.Body).Decode(&artifact); err != nil {
		t.Fatalf("decode artifact response: %v", err)
	}
	if artifact.Status != StatusReady || artifact.URI != "artifact://v1/package.yolo.tar.gz" {
		t.Fatalf("expected storage-backed ready artifact, got %+v", artifact)
	}
	if store.storeCalls != 1 {
		t.Fatalf("expected one storage StoreBuild call, got %d", store.storeCalls)
	}
	if store.uploadCalls != 0 {
		t.Fatalf("expected no upload hook usage, got %d", store.uploadCalls)
	}
}

func TestPresignArtifactFallsBackToDownloadRouteForFilesystemBackedArtifacts(t *testing.T) {
	store := newArtifactStorageStub()
	repo := NewInMemoryRepository()
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID:    1,
		DatasetID:    1,
		SnapshotID:   2,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		URI:          "artifact://v1/package.yolo.tar.gz",
		ManifestURI:  "artifact://v1/manifest.json",
		Checksum:     "sha256:abc123",
		Size:         7,
		Status:       StatusReady,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	svc := NewServiceWithDependencies(repo, nil, nil, store)
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			PresignArtifact: h.PresignArtifact,
		},
	})

	presignReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/1/presign", strings.NewReader(`{"ttl_seconds":60}`))
	presignRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(presignRec, presignReq)
	if presignRec.Code != http.StatusOK {
		t.Fatalf("expected presign success, got %d body=%s", presignRec.Code, presignRec.Body.String())
	}
	if !strings.Contains(presignRec.Body.String(), "/v1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/download") {
		t.Fatalf("expected filesystem-backed presign to use download route, got %s", presignRec.Body.String())
	}
	if strings.Contains(presignRec.Body.String(), "signed.local") {
		t.Fatalf("expected filesystem-backed presign to avoid placeholder signed url, got %s", presignRec.Body.String())
	}
}

func TestCreatePackageRejectsProjectOutsideCallerScope(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		MutationMiddleware: auth.IdentityMiddleware([]int64{2}),
		Artifacts: server.ArtifactRoutes{
			CreatePackage: h.CreatePackage,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"project_id":1,"dataset_id":1,"snapshot_id":1,"format":"yolo"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for create package, got %d body=%s", rec.Code, rec.Body.String())
	}
}
