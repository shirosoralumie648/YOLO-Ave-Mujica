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

func TestOpenAPISnapshotExportDocumentsYoloOnlyRequestFormat(t *testing.T) {
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
	if !strings.Contains(block, "enum: [yolo]") {
		t.Fatalf("expected export path to document yolo-only format, got:\n%s", block)
	}
}

func TestOpenAPIArtifactPackageDocumentsYoloOnlyRequestFormat(t *testing.T) {
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
	if !strings.Contains(block, "enum: [yolo]") {
		t.Fatalf("expected artifact package path to document yolo-only format, got:\n%s", block)
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
	for _, code := range []string{`"404":`, `"422":`} {
		if !strings.Contains(workspaceBlock, code) {
			t.Fatalf("expected workspace path to document %s, got:\n%s", code, workspaceBlock)
		}
	}

	draftBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}/workspace/draft")
	if err != nil {
		t.Fatalf("extract workspace draft path block: %v", err)
	}
	for _, code := range []string{`"404":`, `"409":`, `"422":`} {
		if !strings.Contains(draftBlock, code) {
			t.Fatalf("expected workspace draft path to document %s, got:\n%s", code, draftBlock)
		}
	}

	submitBlock, err := extractOpenAPIPathBlock(raw, "/v1/tasks/{id}/workspace/submit")
	if err != nil {
		t.Fatalf("extract workspace submit path block: %v", err)
	}
	for _, code := range []string{`"404":`, `"409":`, `"422":`} {
		if !strings.Contains(submitBlock, code) {
			t.Fatalf("expected workspace submit path to document %s, got:\n%s", code, submitBlock)
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
