package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStaticBearerMiddlewareRejectsMissingToken(t *testing.T) {
	handler := StaticBearerMiddleware("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStaticBearerMiddlewareAllowsMatchingBearerToken(t *testing.T) {
	handler := StaticBearerMiddleware("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFixedWindowRateLimitMiddlewareRejectsBurstFromSameClient(t *testing.T) {
	handler := FixedWindowRateLimitMiddleware(1, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	req1.RemoteAddr = "198.51.100.10:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusNoContent {
		t.Fatalf("expected first request 204, got %d body=%s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	req2.RemoteAddr = "198.51.100.10:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d body=%s", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header, got headers=%v", rec2.Header())
	}
	if !strings.Contains(rec2.Body.String(), "rate limit exceeded") {
		t.Fatalf("expected rate limit error body, got %s", rec2.Body.String())
	}
}

func TestFixedWindowRateLimitMiddlewareUsesBearerTokenBeforeRemoteAddr(t *testing.T) {
	handler := FixedWindowRateLimitMiddleware(1, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	req1.RemoteAddr = "198.51.100.10:1234"
	req1.Header.Set("Authorization", "Bearer token-a")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusNoContent {
		t.Fatalf("expected first bearer request 204, got %d body=%s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	req2.RemoteAddr = "198.51.100.10:1234"
	req2.Header.Set("Authorization", "Bearer token-b")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNoContent {
		t.Fatalf("expected different bearer token to bypass prior bucket, got %d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestIdentityMiddlewareUsesDefaultProjectScopes(t *testing.T) {
	handler := IdentityMiddleware([]int64{1, 2})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Fatal("expected identity in context")
		}
		if !identity.AllowsProject(1) || !identity.AllowsProject(2) || identity.AllowsProject(3) {
			t.Fatalf("unexpected identity project scopes: %+v", identity)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIdentityMiddlewareOverridesDefaultScopesFromHeader(t *testing.T) {
	handler := IdentityMiddleware([]int64{1})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Fatal("expected identity in context")
		}
		if identity.Actor != "reviewer-1" {
			t.Fatalf("expected actor reviewer-1, got %+v", identity)
		}
		if !identity.AllowsProject(4) || !identity.AllowsProject(5) || identity.AllowsProject(1) {
			t.Fatalf("expected header-based scopes to override defaults, got %+v", identity)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets", nil)
	req.Header.Set(ActorHeader, "reviewer-1")
	req.Header.Set(ProjectScopesHeader, "4,5")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
	}
}
