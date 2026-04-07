package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func newFakeModules() Modules {
	return Modules{
		DataHub: DataHubRoutes{
			CreateDataset:          okHandler,
			ListDatasets:           okHandler,
			GetDatasetDetail:       okHandler,
			GetSnapshotDetail:      okHandler,
			ScanDataset:            okHandler,
			CreateSnapshot:         okHandler,
			ListSnapshots:          okHandler,
			ListItems:              okHandler,
			PresignObject:          okHandler,
			ImportSnapshot:         okHandler,
			CompleteImportSnapshot: okHandler,
		},
		Jobs: JobRoutes{
			CreateZeroShot:     okHandler,
			CreateVideoExtract: okHandler,
			CreateCleaning:     okHandler,
			GetJob:             okHandler,
			ListEvents:         okHandler,
			ReportHeartbeat:    okHandler,
			ReportProgress:     okHandler,
			ReportItemError:    okHandler,
			ReportTerminal:     okHandler,
		},
		Versioning: VersioningRoutes{
			DiffSnapshots: okHandler,
		},
		Review: ReviewRoutes{
			ListCandidates:  okHandler,
			AcceptCandidate: okHandler,
			RejectCandidate: okHandler,
		},
		Publish: PublishRoutes{
			ListCandidates:    okHandler,
			CreateBatch:       okHandler,
			GetBatch:          okHandler,
			ReplaceBatchItems: okHandler,
			ReviewApprove:     okHandler,
			ReviewReject:      okHandler,
			ReviewRework:      okHandler,
			OwnerApprove:      okHandler,
			OwnerReject:       okHandler,
			OwnerRework:       okHandler,
			AddBatchFeedback:  okHandler,
			AddItemFeedback:   okHandler,
			GetWorkspace:      okHandler,
			GetRecord:         okHandler,
		},
		Artifacts: ArtifactRoutes{
			CreatePackage:    okHandler,
			GetArtifact:      okHandler,
			PresignArtifact:  okHandler,
			ResolveArtifact:  okHandler,
			ExportSnapshot:   okHandler,
			CompleteArtifact: okHandler,
			DownloadArtifact: okHandler,
		},
	}
}

func TestMVPRoutesAreRegistered(t *testing.T) {
	srv := NewHTTPServerWithModules(newFakeModules())

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/datasets"},
		{http.MethodGet, "/v1/datasets"},
		{http.MethodGet, "/v1/datasets/1"},
		{http.MethodPost, "/v1/datasets/1/scan"},
		{http.MethodGet, "/v1/datasets/1/items"},
		{http.MethodGet, "/v1/snapshots/1"},
		{http.MethodGet, "/v1/projects/1/overview"},
		{http.MethodGet, "/v1/projects/1/tasks"},
		{http.MethodPost, "/v1/projects/1/tasks"},
		{http.MethodGet, "/v1/tasks/1"},
		{http.MethodPost, "/v1/tasks/1/transition"},
		{http.MethodGet, "/v1/tasks/1/workspace"},
		{http.MethodPut, "/v1/tasks/1/workspace/draft"},
		{http.MethodPost, "/v1/tasks/1/workspace/submit"},
		{http.MethodPost, "/v1/jobs/zero-shot"},
		{http.MethodGet, "/v1/jobs/1"},
		{http.MethodGet, "/v1/jobs/workers"},
		{http.MethodPost, "/v1/snapshots/diff"},
		{http.MethodPost, "/v1/snapshots/1/import"},
		{http.MethodPost, "/internal/snapshots/1/import"},
		{http.MethodPost, "/v1/snapshots/1/export"},
		{http.MethodGet, "/v1/review/candidates"},
		{http.MethodGet, "/v1/publish/candidates?project_id=1"},
		{http.MethodPost, "/v1/publish/batches"},
		{http.MethodGet, "/v1/publish/batches/1"},
		{http.MethodPost, "/v1/publish/batches/1/items"},
		{http.MethodPost, "/v1/publish/batches/1/review-approve"},
		{http.MethodPost, "/v1/publish/batches/1/review-reject"},
		{http.MethodPost, "/v1/publish/batches/1/review-rework"},
		{http.MethodPost, "/v1/publish/batches/1/owner-approve"},
		{http.MethodPost, "/v1/publish/batches/1/owner-reject"},
		{http.MethodPost, "/v1/publish/batches/1/owner-rework"},
		{http.MethodPost, "/v1/publish/batches/1/feedback"},
		{http.MethodPost, "/v1/publish/batches/1/items/1/feedback"},
		{http.MethodGet, "/v1/publish/batches/1/workspace"},
		{http.MethodGet, "/v1/publish/records/1"},
		{http.MethodPost, "/v1/artifacts/packages"},
		{http.MethodGet, "/v1/artifacts/resolve?format=yolo&version=v1"},
		{http.MethodGet, "/v1/artifacts/1/download"},
		{http.MethodPost, "/internal/jobs/1/heartbeat"},
		{http.MethodPost, "/internal/jobs/workers/register"},
		{http.MethodPost, "/internal/jobs/1/progress"},
		{http.MethodPost, "/internal/jobs/1/events"},
		{http.MethodPost, "/internal/jobs/1/complete"},
		{http.MethodPost, "/internal/artifacts/1/complete"},
	}

	for _, tc := range routes {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("route missing: %s %s", tc.method, tc.path)
		}
	}
}

func TestOpenAPIPublicRoutesMatchRegisteredRoutes(t *testing.T) {
	srv := NewHTTPServerWithModules(newFakeModules())

	mux, ok := srv.Handler.(chi.Routes)
	if !ok {
		t.Fatalf("expected chi routes, got %T", srv.Handler)
	}

	registered := map[string]struct{}{}
	if err := chi.Walk(mux, func(method string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !isPublicRoute(route) {
			return nil
		}
		registered[method+" "+route] = struct{}{}
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	documented, err := readOpenAPIPublicRoutes()
	if err != nil {
		t.Fatalf("read openapi routes: %v", err)
	}

	missingFromSpec := diffRouteKeys(registered, documented)
	if len(missingFromSpec) > 0 {
		t.Fatalf("public routes missing from OpenAPI: %s", strings.Join(missingFromSpec, ", "))
	}

	staleInSpec := diffRouteKeys(documented, registered)
	if len(staleInSpec) > 0 {
		t.Fatalf("OpenAPI documents routes not registered by server: %s", strings.Join(staleInSpec, ", "))
	}
}

func TestParseOpenAPIPublicRoutesRejectsDuplicatePathEntries(t *testing.T) {
	_, err := parseOpenAPIPublicRoutes(`
paths:
  /v1/datasets:
    get:
      summary: list
  /v1/datasets:
    post:
      summary: create
`)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate path") {
		t.Fatalf("expected duplicate path error, got %v", err)
	}
}

func TestParseOpenAPIPublicRoutesRejectsDuplicateMethodEntries(t *testing.T) {
	_, err := parseOpenAPIPublicRoutes(`
paths:
  /v1/datasets:
    get:
      summary: list
    get:
      summary: duplicate list
`)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate method") {
		t.Fatalf("expected duplicate method error, got %v", err)
	}
}

func TestOpenAPISnapshotExportDocumentsSupportedRequestFormats(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	block, err := extractOpenAPIPathBlock(raw, "/v1/snapshots/{id}/export")
	if err != nil {
		t.Fatalf("extract export path block: %v", err)
	}
	if !strings.Contains(block, "requestBody:") {
		t.Fatalf("expected export path to declare a requestBody, got:\n%s", block)
	}
	if !strings.Contains(block, "required: [dataset_id, format, version]") {
		t.Fatalf("expected export path to require dataset_id/format/version, got:\n%s", block)
	}
	if !strings.Contains(block, "enum: [yolo, coco]") {
		t.Fatalf("expected export path to document yolo and coco formats, got:\n%s", block)
	}
}

func TestOpenAPIArtifactPackageDocumentsSupportedRequestFormats(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	block, err := extractOpenAPIPathBlock(raw, "/v1/artifacts/packages")
	if err != nil {
		t.Fatalf("extract artifact package path block: %v", err)
	}
	if !strings.Contains(block, "requestBody:") {
		t.Fatalf("expected artifact package path to declare a requestBody, got:\n%s", block)
	}
	if !strings.Contains(block, "required: [dataset_id, snapshot_id, format, version]") {
		t.Fatalf("expected artifact package path to require dataset_id/snapshot_id/format/version, got:\n%s", block)
	}
	if !strings.Contains(block, "enum: [yolo, coco]") {
		t.Fatalf("expected artifact package path to document yolo and coco formats, got:\n%s", block)
	}
}

func TestOpenAPIDefinesBearerAuthAndTraceHeaders(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	for _, needle := range []string{
		"bearerAuth:",
		"RequestIdHeader:",
		"X-Request-Id",
		"X-Correlation-Id",
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("expected openapi document to contain %q, got:\n%s", needle, raw)
		}
	}
}

func TestOpenAPIJobCreationRoutesDocumentRequestIDAndBearerAuth(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	for _, path := range []string{
		"/v1/snapshots/{id}/import",
		"/v1/snapshots/{id}/export",
		"/v1/jobs/zero-shot",
		"/v1/jobs/video-extract",
		"/v1/jobs/cleaning",
		"/v1/artifacts/packages",
	} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract path block %s: %v", path, err)
		}
		if !strings.Contains(block, `$ref: '#/components/parameters/RequestIdHeader'`) {
			t.Fatalf("expected %s to document X-Request-Id, got:\n%s", path, block)
		}
		if !strings.Contains(block, "security:") || !strings.Contains(block, "bearerAuth") {
			t.Fatalf("expected %s to document bearer auth, got:\n%s", path, block)
		}
	}
}

func TestOpenAPIProjectScopedMutationRoutesDocument403FailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	for _, path := range []string{
		"/v1/datasets",
		"/v1/datasets/{id}/scan",
		"/v1/datasets/{id}/snapshots",
		"/v1/objects/presign",
		"/v1/snapshots/{id}/import",
		"/v1/jobs/zero-shot",
		"/v1/jobs/video-extract",
		"/v1/jobs/cleaning",
		"/v1/review/candidates/{id}/accept",
		"/v1/review/candidates/{id}/reject",
		"/v1/artifacts/packages",
		"/v1/snapshots/{id}/export",
		"/v1/artifacts/{id}/presign",
	} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"403":`) {
			t.Fatalf("expected %s to document 403 forbidden responses, got:\n%s", path, block)
		}
	}
}

func TestOpenAPIDocumentsProjectScopeHeaders(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	for _, needle := range []string{
		"AUTH_DEFAULT_PROJECT_IDS",
		"X-Project-Scopes",
		"X-Actor",
	} {
		if !strings.Contains(raw, needle) {
			t.Fatalf("expected openapi document to contain %q, got:\n%s", needle, raw)
		}
	}
}

func TestOpenAPIWorkspaceRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	workspaceBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}/workspace")
	if err != nil {
		t.Fatalf("extract workspace path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`, `"422":`} {
		if !strings.Contains(workspaceBlock, code) {
			t.Fatalf("expected workspace path to document %s, got:\n%s", code, workspaceBlock)
		}
	}

	draftBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}/workspace/draft")
	if err != nil {
		t.Fatalf("extract workspace draft path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`, `"409":`, `"422":`} {
		if !strings.Contains(draftBlock, code) {
			t.Fatalf("expected workspace draft path to document %s, got:\n%s", code, draftBlock)
		}
	}

	submitBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}/workspace/submit")
	if err != nil {
		t.Fatalf("extract workspace submit path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`, `"409":`, `"422":`} {
		if !strings.Contains(submitBlock, code) {
			t.Fatalf("expected workspace submit path to document %s, got:\n%s", code, submitBlock)
		}
	}
}

func TestOpenAPIOverviewAndTaskRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	overviewBlock, err := extractOpenAPIPathBlock(raw, "/v1/projects/{id}/overview")
	if err != nil {
		t.Fatalf("extract overview path block: %v", err)
	}
	if !strings.Contains(overviewBlock, `"400":`) {
		t.Fatalf("expected overview path to document 400 failures, got:\n%s", overviewBlock)
	}

	tasksBlock, err := extractOpenAPIPathBlock(raw, "/v1/projects/{id}/tasks")
	if err != nil {
		t.Fatalf("extract tasks path block: %v", err)
	}
	if !strings.Contains(tasksBlock, `"400":`) {
		t.Fatalf("expected project tasks path to document 400 failures, got:\n%s", tasksBlock)
	}

	taskDetailBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}")
	if err != nil {
		t.Fatalf("extract task detail path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`} {
		if !strings.Contains(taskDetailBlock, code) {
			t.Fatalf("expected task detail path to document %s failures, got:\n%s", code, taskDetailBlock)
		}
	}

	transitionBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}/transition")
	if err != nil {
		t.Fatalf("extract task transition path block: %v", err)
	}
	if !strings.Contains(transitionBlock, `"400":`) {
		t.Fatalf("expected task transition path to document 400 failures, got:\n%s", transitionBlock)
	}
}

func TestOpenAPIDataHubRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	createDatasetBlock, err := extractOpenAPIPathBlock(raw, "/v1/datasets")
	if err != nil {
		t.Fatalf("extract datasets path block: %v", err)
	}
	if !strings.Contains(createDatasetBlock, `"400":`) {
		t.Fatalf("expected datasets path to document 400 failures for create, got:\n%s", createDatasetBlock)
	}

	datasetDetailBlock, err := extractOpenAPIPathBlock(raw, "/v1/datasets/{id}")
	if err != nil {
		t.Fatalf("extract dataset detail path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`} {
		if !strings.Contains(datasetDetailBlock, code) {
			t.Fatalf("expected dataset detail path to document %s failures, got:\n%s", code, datasetDetailBlock)
		}
	}

	for _, path := range []string{"/v1/datasets/{id}/scan", "/v1/datasets/{id}/snapshots", "/v1/datasets/{id}/items", "/v1/objects/presign"} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract datahub path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"400":`) {
			t.Fatalf("expected %s to document 400 failures, got:\n%s", path, block)
		}
	}

	snapshotDetailBlock, err := extractOpenAPIPathBlock(raw, "/v1/snapshots/{id}")
	if err != nil {
		t.Fatalf("extract snapshot detail path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`} {
		if !strings.Contains(snapshotDetailBlock, code) {
			t.Fatalf("expected snapshot detail path to document %s failures, got:\n%s", code, snapshotDetailBlock)
		}
	}
}

func TestOpenAPISnapshotImportDocumentsFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	block, err := extractOpenAPIPathBlock(raw, "/v1/snapshots/{id}/import")
	if err != nil {
		t.Fatalf("extract snapshot import path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`} {
		if !strings.Contains(block, code) {
			t.Fatalf("expected snapshot import path to document %s, got:\n%s", code, block)
		}
	}
}

func TestOpenAPISnapshotExportDocumentsFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	block, err := extractOpenAPIPathBlock(raw, "/v1/snapshots/{id}/export")
	if err != nil {
		t.Fatalf("extract snapshot export path block: %v", err)
	}
	if !strings.Contains(block, `"400":`) {
		t.Fatalf("expected snapshot export path to document 400 failures, got:\n%s", block)
	}
}

func TestOpenAPIJobRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	for _, path := range []string{"/v1/jobs/zero-shot", "/v1/jobs/video-extract", "/v1/jobs/cleaning"} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract job create path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"400":`) {
			t.Fatalf("expected %s to document 400 failures, got:\n%s", path, block)
		}
	}

	for _, path := range []string{"/v1/jobs/{job_id}", "/v1/jobs/{job_id}/events"} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract job read path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"400":`) {
			t.Fatalf("expected %s to document 400 failures, got:\n%s", path, block)
		}
	}

	jobBlock, err := extractOpenAPIPathBlock(raw, "/v1/jobs/{job_id}")
	if err != nil {
		t.Fatalf("extract job detail path block: %v", err)
	}
	if !strings.Contains(jobBlock, `"404":`) {
		t.Fatalf("expected /v1/jobs/{job_id} to document 404 failures, got:\n%s", jobBlock)
	}
}

func TestOpenAPIPublishRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	candidatesBlock, err := extractOpenAPIPathBlock(raw, "/v1/publish/candidates")
	if err != nil {
		t.Fatalf("extract publish candidates path block: %v", err)
	}
	if !strings.Contains(candidatesBlock, `"400":`) {
		t.Fatalf("expected publish candidates path to document 400 failures, got:\n%s", candidatesBlock)
	}

	createBlock, err := extractOpenAPIPathBlock(raw, "/v1/publish/batches")
	if err != nil {
		t.Fatalf("extract publish batches path block: %v", err)
	}
	if !strings.Contains(createBlock, `"400":`) {
		t.Fatalf("expected publish batches path to document 400 failures, got:\n%s", createBlock)
	}

	for _, path := range []string{"/v1/publish/batches/{id}", "/v1/publish/batches/{id}/workspace", "/v1/publish/records/{id}"} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract publish read path block %s: %v", path, err)
		}
		for _, code := range []string{`"400":`, `"404":`} {
			if !strings.Contains(block, code) {
				t.Fatalf("expected %s to document %s failures, got:\n%s", path, code, block)
			}
		}
	}
}

func TestOpenAPIPublishMutationRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	for _, path := range []string{
		"/v1/publish/batches/{id}/items",
		"/v1/publish/batches/{id}/review-approve",
		"/v1/publish/batches/{id}/review-reject",
		"/v1/publish/batches/{id}/review-rework",
		"/v1/publish/batches/{id}/owner-approve",
		"/v1/publish/batches/{id}/owner-reject",
		"/v1/publish/batches/{id}/owner-rework",
		"/v1/publish/batches/{id}/feedback",
		"/v1/publish/batches/{id}/items/{itemId}/feedback",
	} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract publish mutation path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"400":`) {
			t.Fatalf("expected %s to document 400 failures, got:\n%s", path, block)
		}
	}
}

func TestOpenAPIPublicMutationRoutesDocument429FailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	for _, path := range []string{
		"/v1/datasets",
		"/v1/datasets/{id}/scan",
		"/v1/datasets/{id}/snapshots",
		"/v1/objects/presign",
		"/v1/snapshots/{id}/import",
		"/v1/projects/{id}/tasks",
		"/v1/tasks/{id}/transition",
		"/v1/tasks/{id}/workspace/draft",
		"/v1/tasks/{id}/workspace/submit",
		"/v1/jobs/zero-shot",
		"/v1/jobs/video-extract",
		"/v1/jobs/cleaning",
		"/v1/snapshots/diff",
		"/v1/review/candidates/{id}/accept",
		"/v1/review/candidates/{id}/reject",
		"/v1/publish/batches",
		"/v1/publish/batches/{id}/items",
		"/v1/publish/batches/{id}/review-approve",
		"/v1/publish/batches/{id}/review-reject",
		"/v1/publish/batches/{id}/review-rework",
		"/v1/publish/batches/{id}/owner-approve",
		"/v1/publish/batches/{id}/owner-reject",
		"/v1/publish/batches/{id}/owner-rework",
		"/v1/publish/batches/{id}/feedback",
		"/v1/publish/batches/{id}/items/{itemId}/feedback",
		"/v1/artifacts/packages",
		"/v1/snapshots/{id}/export",
		"/v1/artifacts/{id}/presign",
	} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract mutation path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"429":`) {
			t.Fatalf("expected %s to document 429 failures, got:\n%s", path, block)
		}
	}
}

func TestOpenAPISnapshotDiffDocumentsFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}
	block, err := extractOpenAPIPathBlock(raw, "/v1/snapshots/diff")
	if err != nil {
		t.Fatalf("extract snapshot diff path block: %v", err)
	}
	if !strings.Contains(block, `"400":`) {
		t.Fatalf("expected snapshot diff path to document 400 failures, got:\n%s", block)
	}
}

func TestOpenAPIReviewRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	for _, path := range []string{"/v1/review/candidates/{id}/accept", "/v1/review/candidates/{id}/reject"} {
		block, err := extractOpenAPIPathBlock(raw, path)
		if err != nil {
			t.Fatalf("extract review path block %s: %v", path, err)
		}
		if !strings.Contains(block, `"400":`) {
			t.Fatalf("expected %s to document 400 failures, got:\n%s", path, block)
		}
	}
}

func TestOpenAPIArtifactRoutesDocumentFailureResponses(t *testing.T) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		t.Fatalf("read openapi document: %v", err)
	}

	getBlock, err := extractOpenAPIPathBlock(raw, "/v1/artifacts/{id}")
	if err != nil {
		t.Fatalf("extract artifact detail path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`} {
		if !strings.Contains(getBlock, code) {
			t.Fatalf("expected artifact detail path to document %s failures, got:\n%s", code, getBlock)
		}
	}

	downloadBlock, err := extractOpenAPIPathBlock(raw, "/v1/artifacts/{id}/download")
	if err != nil {
		t.Fatalf("extract artifact download path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`, `"409":`} {
		if !strings.Contains(downloadBlock, code) {
			t.Fatalf("expected artifact download path to document %s failures, got:\n%s", code, downloadBlock)
		}
	}

	presignBlock, err := extractOpenAPIPathBlock(raw, "/v1/artifacts/{id}/presign")
	if err != nil {
		t.Fatalf("extract artifact presign path block: %v", err)
	}
	for _, code := range []string{`"400":`, `"404":`, `"409":`} {
		if !strings.Contains(presignBlock, code) {
			t.Fatalf("expected artifact presign path to document %s failures, got:\n%s", code, presignBlock)
		}
	}
}

func readOpenAPIPublicRoutes() (map[string]struct{}, error) {
	raw, err := readOpenAPIDocument()
	if err != nil {
		return nil, err
	}
	return parseOpenAPIPublicRoutes(raw)
}

func readOpenAPIDocument() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	openapiPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "api", "openapi", "mvp.yaml")
	raw, err := os.ReadFile(openapiPath)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func extractOpenAPIPathBlock(raw string, path string) (string, error) {
	lines := strings.Split(raw, "\n")
	header := "  " + path + ":"
	start := -1
	for idx, line := range lines {
		if line == header {
			start = idx
			break
		}
	}
	if start < 0 {
		return "", &openAPIParseError{kind: "missing path", value: path}
	}
	end := len(lines)
	for idx := start + 1; idx < len(lines); idx++ {
		if strings.HasPrefix(lines[idx], "  /") {
			end = idx
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), nil
}

func parseOpenAPIPublicRoutes(raw string) (map[string]struct{}, error) {
	routes := map[string]struct{}{}
	seenPaths := map[string]struct{}{}
	seenMethods := map[string]map[string]struct{}{}
	var currentPath string
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "  /"):
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			if _, ok := seenPaths[currentPath]; ok {
				return nil, &openAPIParseError{kind: "duplicate path", value: currentPath}
			}
			seenPaths[currentPath] = struct{}{}
		case currentPath != "" && strings.HasPrefix(line, "    "):
			trimmed := strings.TrimSpace(line)
			if !strings.HasSuffix(trimmed, ":") {
				continue
			}
			method := strings.ToUpper(strings.TrimSuffix(trimmed, ":"))
			switch method {
			case http.MethodGet, http.MethodPost, http.MethodPut:
				if _, ok := seenMethods[currentPath]; !ok {
					seenMethods[currentPath] = map[string]struct{}{}
				}
				if _, ok := seenMethods[currentPath][method]; ok {
					return nil, &openAPIParseError{kind: "duplicate method", value: method + " " + currentPath}
				}
				seenMethods[currentPath][method] = struct{}{}
				if isPublicRoute(currentPath) {
					routes[method+" "+currentPath] = struct{}{}
				}
			}
		}
	}
	return routes, nil
}

func isPublicRoute(route string) bool {
	return route == "/healthz" || route == "/readyz" || strings.HasPrefix(route, "/v1/")
}

func diffRouteKeys(want, have map[string]struct{}) []string {
	missing := make([]string, 0)
	for key := range want {
		if _, ok := have[key]; ok {
			continue
		}
		missing = append(missing, key)
	}
	slices.Sort(missing)
	return missing
}

type openAPIParseError struct {
	kind  string
	value string
}

func (e *openAPIParseError) Error() string {
	return e.kind + ": " + e.value
}
