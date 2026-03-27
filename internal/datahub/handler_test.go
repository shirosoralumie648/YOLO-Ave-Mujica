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
