package datahub_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/server"
)

func newTestServerWithFakePresigner() *server.HTTPServer {
	svc := datahub.NewService(func(_ int64, _ string, _ int) (string, error) {
		return "https://signed.local/object", nil
	})
	h := datahub.NewHandler(svc)
	return server.NewHTTPServerWithDataHub(h)
}

func TestPresignEndpointReturnsURL(t *testing.T) {
	srv := newTestServerWithFakePresigner()
	req := httptest.NewRequest(http.MethodPost, "/v1/objects/presign", strings.NewReader(`{"dataset_id":1,"object_key":"train/a.jpg"}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://") {
		t.Fatalf("expected signed URL, got %s", rec.Body.String())
	}
}

func TestScanAndListItems(t *testing.T) {
	srv := newTestServerWithFakePresigner()

	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest(http.MethodPost, "/v1/datasets", strings.NewReader(`{"project_id":1,"name":"d1","bucket":"bkt","prefix":"train"}`))
	srv.Handler.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusOK {
		t.Fatalf("create dataset failed: %d body=%s", recCreate.Code, recCreate.Body.String())
	}

	recScan := httptest.NewRecorder()
	reqScan := httptest.NewRequest(http.MethodPost, "/v1/datasets/1/scan", strings.NewReader(`{"object_keys":["train/a.jpg"]}`))
	srv.Handler.ServeHTTP(recScan, reqScan)
	if recScan.Code != http.StatusOK {
		t.Fatalf("scan failed: %d body=%s", recScan.Code, recScan.Body.String())
	}

	recItems := httptest.NewRecorder()
	reqItems := httptest.NewRequest(http.MethodGet, "/v1/datasets/1/items", nil)
	srv.Handler.ServeHTTP(recItems, reqItems)
	if recItems.Code != http.StatusOK {
		t.Fatalf("list items failed: %d body=%s", recItems.Code, recItems.Body.String())
	}
	if !strings.Contains(recItems.Body.String(), "train/a.jpg") {
		t.Fatalf("expected indexed object in items list, got %s", recItems.Body.String())
	}
}
