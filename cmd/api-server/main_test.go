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
	"yolo-ave-mujica/internal/publish"
	"yolo-ave-mujica/internal/review"
	"yolo-ave-mujica/internal/server"
	"yolo-ave-mujica/internal/tasks"
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

func TestBuildModulesWithHandlersUsesInjectedReviewPublishAndArtifacts(t *testing.T) {
	reviewSvc := review.NewService()
	reviewSvc.SeedCandidate(review.Candidate{ID: 10, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})
	reviewHandler := review.NewHandler(reviewSvc)

	publishSvc := publish.NewService(publish.NewInMemoryRepository(), tasks.NewService(tasks.NewInMemoryRepository()))
	publishHandler := publish.NewHandler(publishSvc)

	artifactSvc := artifacts.NewService()
	artifact, err := artifactSvc.CreatePackageJob(artifacts.PackageRequest{
		DatasetID:  1,
		SnapshotID: 2,
		Format:     "yolo",
		Version:    "v1",
	})
	if err != nil {
		t.Fatalf("create artifact fixture: %v", err)
	}
	artifactHandler := artifacts.NewHandler(artifactSvc)

	modules := buildModulesWithHandlers(reviewHandler, publishHandler, artifactHandler)
	srv := server.NewHTTPServerWithModules(modules)

	reviewReq := httptest.NewRequest(http.MethodGet, "/v1/review/candidates", nil)
	reviewRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK || !strings.Contains(reviewRec.Body.String(), `"id":10`) {
		t.Fatalf("expected injected review handler to serve candidate, got %d %s", reviewRec.Code, reviewRec.Body.String())
	}

	publishReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches", strings.NewReader(`{
		"project_id": 1,
		"snapshot_id": 2,
		"source": "suggested",
		"items": [{
			"candidate_id": 101,
			"task_id": 99,
			"dataset_id": 1,
			"snapshot_id": 2,
			"item_payload": {"task": {"id": 99}}
		}]
	}`))
	publishRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusCreated {
		t.Fatalf("expected injected publish handler to create batch, got %d %s", publishRec.Code, publishRec.Body.String())
	}

	artifactReq := httptest.NewRequest(http.MethodGet, "/v1/artifacts/1", nil)
	artifactRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(artifactRec, artifactReq)
	if artifactRec.Code != http.StatusOK || !strings.Contains(artifactRec.Body.String(), `"id":`+strconv.FormatInt(artifact.ID, 10)) {
		t.Fatalf("expected injected artifact handler to serve artifact, got %d %s", artifactRec.Code, artifactRec.Body.String())
	}
}

func TestNewTestModulesLeavesTaskAndOverviewRoutesUnwired(t *testing.T) {
	srv := server.NewHTTPServerWithModules(newTestModules())

	datasetsReq := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	datasetsRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(datasetsRec, datasetsReq)
	if datasetsRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for datasets browse route before module wiring, got %d", datasetsRec.Code)
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/v1/snapshots/1", nil)
	snapshotRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for snapshot detail route before module wiring, got %d", snapshotRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/1/overview", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 before task and overview wiring, got %d", rec.Code)
	}

	transitionReq := httptest.NewRequest(http.MethodPost, "/v1/tasks/1/transition", strings.NewReader(`{"status":"ready"}`))
	transitionRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(transitionRec, transitionReq)
	if transitionRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for transition route before task wiring, got %d", transitionRec.Code)
	}

	workspaceReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/1/workspace", nil)
	workspaceRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for annotation workspace route before wiring, got %d", workspaceRec.Code)
	}
}
