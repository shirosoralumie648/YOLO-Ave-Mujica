package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
