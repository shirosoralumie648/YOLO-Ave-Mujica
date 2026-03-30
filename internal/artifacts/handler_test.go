package artifacts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/server"
)

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
