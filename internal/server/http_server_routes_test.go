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
		DataHub: DataHubRoutes{
			CreateDataset:  okHandler,
			ScanDataset:    okHandler,
			CreateSnapshot: okHandler,
			ListSnapshots:  okHandler,
			ListItems:      okHandler,
			PresignObject:  okHandler,
		},
		Jobs: JobRoutes{
			CreateZeroShot:     okHandler,
			CreateVideoExtract: okHandler,
			CreateCleaning:     okHandler,
			GetJob:             okHandler,
			ListEvents:         okHandler,
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
		{http.MethodPost, "/v1/datasets/1/scan"},
		{http.MethodGet, "/v1/datasets/1/items"},
		{http.MethodPost, "/v1/jobs/zero-shot"},
		{http.MethodGet, "/v1/jobs/1"},
		{http.MethodPost, "/v1/snapshots/diff"},
		{http.MethodGet, "/v1/review/candidates"},
		{http.MethodPost, "/v1/artifacts/packages"},
		{http.MethodGet, "/v1/artifacts/resolve?format=yolo&version=v1"},
		{http.MethodGet, "/v1/artifacts/1/download"},
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
