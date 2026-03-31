package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func newFakeModules() Modules {
	return Modules{
		Overview: OverviewRoutes{
			GetProjectOverview: okHandler,
		},
		Tasks: TaskRoutes{
			ListProjectTasks:  okHandler,
			CreateProjectTask: okHandler,
			GetTask:           okHandler,
		},
		DataHub: DataHubRoutes{
			CreateDataset:          okHandler,
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
		{http.MethodGet, "/v1/projects/1/overview"},
		{http.MethodGet, "/v1/projects/1/tasks"},
		{http.MethodPost, "/v1/projects/1/tasks"},
		{http.MethodGet, "/v1/tasks/1"},
		{http.MethodPost, "/v1/datasets/1/scan"},
		{http.MethodGet, "/v1/datasets/1/items"},
		{http.MethodPost, "/v1/jobs/zero-shot"},
		{http.MethodGet, "/v1/jobs/1"},
		{http.MethodPost, "/v1/snapshots/diff"},
		{http.MethodPost, "/v1/snapshots/1/import"},
		{http.MethodPost, "/internal/snapshots/1/import"},
		{http.MethodPost, "/v1/snapshots/1/export"},
		{http.MethodGet, "/v1/review/candidates"},
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
