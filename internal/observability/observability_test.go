package observability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMiddlewareAssignsTraceIDLogsJSONAndRecordsMetrics(t *testing.T) {
	metrics := NewMetrics()
	var logBuf bytes.Buffer

	router := chi.NewRouter()
	router.Use(Middleware(NewJSONLogger(&logBuf), metrics))
	router.Get("/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	traceID := rec.Header().Get(TraceIDHeader)
	if traceID == "" {
		t.Fatal("expected middleware to assign trace id header")
	}
	if rec.Header().Get(CorrelationIDHeader) != traceID {
		t.Fatalf("expected correlation id header to mirror trace id, got trace=%q correlation=%q", traceID, rec.Header().Get(CorrelationIDHeader))
	}

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &entry); err != nil {
		t.Fatalf("expected JSON log line, got err=%v body=%q", err, logBuf.String())
	}
	if entry["trace_id"] != traceID {
		t.Fatalf("expected log trace_id %q, got %+v", traceID, entry)
	}
	if entry["route"] != "/v1/healthz" || entry["method"] != http.MethodGet {
		t.Fatalf("unexpected log entry route or method: %+v", entry)
	}
	if entry["status"] != float64(http.StatusCreated) {
		t.Fatalf("expected status %d, got %+v", http.StatusCreated, entry)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics handler 200, got %d body=%s", metricsRec.Code, metricsRec.Body.String())
	}
	if !strings.Contains(metricsRec.Body.String(), `yolo_http_requests_total{method="GET",route="/v1/healthz",status_class="2xx"} 1`) {
		t.Fatalf("expected http request counter in metrics output, got:\n%s", metricsRec.Body.String())
	}
}

func TestMiddlewarePreservesIncomingTraceID(t *testing.T) {
	metrics := NewMetrics()

	router := chi.NewRouter()
	router.Use(Middleware(nil, metrics))
	router.Get("/v1/datasets", func(w http.ResponseWriter, r *http.Request) {
		if got := TraceIDFromContext(r.Context()); got != "trace-smoke-123" {
			t.Fatalf("expected trace id in request context, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	req.Header.Set(TraceIDHeader, "trace-smoke-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Header().Get(TraceIDHeader) != "trace-smoke-123" {
		t.Fatalf("expected trace id header to be preserved, got %q", rec.Header().Get(TraceIDHeader))
	}
}
