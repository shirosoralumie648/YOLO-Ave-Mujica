package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"yolo-ave-mujica/internal/auth"
)

func TestHealthz(t *testing.T) {
	srv := NewHTTPServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body ok, got %q", rec.Body.String())
	}
}

func TestReadyzWithoutChecks(t *testing.T) {
	srv := NewHTTPServer()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ready" {
		t.Fatalf("expected body ready, got %q", rec.Body.String())
	}
}

func TestReadyzReturnsUnavailableWhenDependencyCheckFails(t *testing.T) {
	srv := NewHTTPServerWithModules(Modules{
		ReadyChecks: []ReadyCheck{
			func(context.Context) error { return errors.New("redis unavailable") },
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "redis unavailable") {
		t.Fatalf("expected readiness error in body, got %q", rec.Body.String())
	}
}

func TestPublicMutationRoutesRequireBearerTokenWhenConfigured(t *testing.T) {
	srv := NewHTTPServerWithModules(Modules{
		MutationMiddleware: auth.StaticBearerMiddleware("secret-token"),
		DataHub: DataHubRoutes{
			CreateDataset: okHandler,
			ListDatasets:  okHandler,
		},
		Jobs: JobRoutes{
			ReportHeartbeat: okHandler,
		},
	})

	postReq := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	postRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated public mutation to return 401, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected public read route to remain open, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	internalReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/1/heartbeat", nil)
	internalRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(internalRec, internalReq)
	if internalRec.Code != http.StatusOK {
		t.Fatalf("expected internal callback route to remain open, got %d body=%s", internalRec.Code, internalRec.Body.String())
	}

	authorizedReq := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	authorizedReq.Header.Set("Authorization", "Bearer secret-token")
	authorizedRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(authorizedRec, authorizedReq)
	if authorizedRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated public mutation to pass, got %d body=%s", authorizedRec.Code, authorizedRec.Body.String())
	}
}

func TestPublicMutationRoutesApplyRateLimitWithoutAffectingReadsOrInternalCallbacks(t *testing.T) {
	srv := NewHTTPServerWithModules(Modules{
		MutationMiddleware: func(next http.Handler) http.Handler {
			return auth.StaticBearerMiddleware("secret-token")(auth.FixedWindowRateLimitMiddleware(1, time.Minute)(next))
		},
		DataHub: DataHubRoutes{
			CreateDataset: okHandler,
			ListDatasets:  okHandler,
		},
		Jobs: JobRoutes{
			ReportHeartbeat: okHandler,
		},
	})

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	firstReq.RemoteAddr = "198.51.100.10:1234"
	firstReq.Header.Set("Authorization", "Bearer secret-token")
	firstRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first authenticated mutation 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	secondReq.RemoteAddr = "198.51.100.10:1234"
	secondReq.Header.Set("Authorization", "Bearer secret-token")
	secondRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second authenticated mutation 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	getReq.RemoteAddr = "198.51.100.10:1234"
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected read route to stay open, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	internalReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/1/heartbeat", nil)
	internalReq.RemoteAddr = "198.51.100.10:1234"
	internalRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(internalRec, internalReq)
	if internalRec.Code != http.StatusOK {
		t.Fatalf("expected internal callback route to stay unthrottled, got %d body=%s", internalRec.Code, internalRec.Body.String())
	}
}

func TestMetricsRouteServesConfiguredHandler(t *testing.T) {
	srv := NewHTTPServerWithModules(Modules{
		MetricsHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("metric_value 1\n"))
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics route 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "metric_value 1\n" {
		t.Fatalf("unexpected metrics body %q", rec.Body.String())
	}
}

func TestHTTPMiddlewareRunsForHealthRoutes(t *testing.T) {
	srv := NewHTTPServerWithModules(Modules{
		HTTPMiddleware: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Test-Middleware", "applied")
				next.ServeHTTP(w, r)
			})
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Test-Middleware") != "applied" {
		t.Fatalf("expected global http middleware to run, got headers=%v", rec.Header())
	}
}
