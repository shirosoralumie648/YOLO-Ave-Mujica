package artifacts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/server"
)

type resolveRepoStub struct {
	byDataset map[string]Artifact
	resolved  Artifact
}

func (r resolveRepoStub) Create(a Artifact) (Artifact, error) {
	return a, nil
}

func (r resolveRepoStub) Get(id int64) (Artifact, bool) {
	if r.resolved.ID == id {
		return r.resolved, true
	}
	return Artifact{}, false
}

func (r resolveRepoStub) FindByDatasetFormatVersion(dataset, format, version string) (Artifact, bool) {
	if dataset != "" {
		if artifact, ok := r.byDataset[dataset]; ok && artifact.Format == format && artifact.Version == version {
			return artifact, true
		}
	}
	if r.resolved.Format == format && r.resolved.Version == version {
		return r.resolved, true
	}
	return Artifact{}, false
}

func (r resolveRepoStub) UpdateReady(id int64, uri, manifestURI, checksum string, size int64) error {
	return nil
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
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "job_id") {
		t.Fatalf("expected async package response, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePackageReturnsArtifactIDThatCanBeFetchedAndPresigned(t *testing.T) {
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
	if createRec.Code != http.StatusOK {
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

	presignReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/1/presign", strings.NewReader(`{"ttl_seconds":60}`))
	presignRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(presignRec, presignReq)
	if presignRec.Code != http.StatusOK || !strings.Contains(presignRec.Body.String(), "https://signed.local/artifacts/1") {
		t.Fatalf("presign artifact failed: %d body=%s", presignRec.Code, presignRec.Body.String())
	}
}

func TestResolveArtifactByFormatAndVersion(t *testing.T) {
	svc := NewService()
	h := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Artifacts: server.ArtifactRoutes{
			CreatePackage:   h.CreatePackage,
			GetArtifact:     h.GetArtifact,
			PresignArtifact: h.PresignArtifact,
			ResolveArtifact: h.ResolveArtifact,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo","version":"v1"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create package failed: %d body=%s", createRec.Code, createRec.Body.String())
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

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"job_id"`) || !strings.Contains(rec.Body.String(), `"artifact_id"`) {
		t.Fatalf("expected job and artifact ids, got %s", rec.Body.String())
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

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
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
	if createRec.Code != http.StatusOK {
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
	if createRec.Code != http.StatusOK {
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

type artifactStorageStub struct {
	uploads map[string][]byte
}

func newArtifactStorageStub() *artifactStorageStub {
	return &artifactStorageStub{uploads: make(map[string][]byte)}
}

func (s *artifactStorageStub) upload(uri string, body []byte, _ string) (int64, error) {
	s.uploads[uri] = append([]byte(nil), body...)
	return int64(len(body)), nil
}

func (s *artifactStorageStub) presign(uri string, ttlSeconds int) (string, error) {
	return "http://download.local/" + strings.TrimPrefix(uri, "s3://"), nil
}
