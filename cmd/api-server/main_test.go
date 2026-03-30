package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yolo-ave-mujica/internal/artifacts"
	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/review"
	"yolo-ave-mujica/internal/server"
)

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:        "127.0.0.1:0",
		ShutdownTimeout: 100 * time.Millisecond,
	}
}

func newTestModules() server.Modules {
	return server.Modules{}
}

func TestRunStopsAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, testConfig(), newTestModules()); err != nil {
		t.Fatalf("expected canceled startup to shut down cleanly, got %v", err)
	}
}

func TestStartBackgroundLoopInvokesTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	done := make(chan struct{})

	startBackgroundLoop(ctx, 5*time.Millisecond, func(time.Time) error {
		if calls.Add(1) == 1 {
			close(done)
		}
		return nil
	})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected background loop to invoke tick")
	}
}

func TestBuildModulesWithHandlersUsesInjectedReviewAndArtifacts(t *testing.T) {
	reviewSvc := review.NewService()
	reviewSvc.SeedCandidate(review.Candidate{ID: 10, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})
	reviewHandler := review.NewHandler(reviewSvc)

	artifactSvc := artifacts.NewService()
	_, artifactID, err := artifactSvc.CreatePackageJob(artifacts.PackageRequest{
		DatasetID:  1,
		SnapshotID: 2,
		Format:     "yolo",
		Version:    "v1",
	})
	if err != nil {
		t.Fatalf("create artifact fixture: %v", err)
	}
	artifactHandler := artifacts.NewHandler(artifactSvc)

	modules := buildModulesWithHandlers(reviewHandler, artifactHandler)
	srv := server.NewHTTPServerWithModules(modules)

	reviewReq := httptest.NewRequest(http.MethodGet, "/v1/review/candidates", nil)
	reviewRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK || !strings.Contains(reviewRec.Body.String(), `"id":10`) {
		t.Fatalf("expected injected review handler to serve candidate, got %d %s", reviewRec.Code, reviewRec.Body.String())
	}

	artifactReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1", nil)
	artifactRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(artifactRec, artifactReq)
	if artifactRec.Code != http.StatusOK || !strings.Contains(artifactRec.Body.String(), `"id":`+strconv.FormatInt(artifactID, 10)) {
		t.Fatalf("expected injected artifact handler to serve artifact, got %d %s", artifactRec.Code, artifactRec.Body.String())
	}
}
