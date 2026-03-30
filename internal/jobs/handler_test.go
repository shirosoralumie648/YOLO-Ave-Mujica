package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestCreateGPUJobPublishesToGPULane(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-1",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job returned error: %v", err)
	}
	if got := pub.LastLane(); got != "jobs:gpu" {
		t.Fatalf("expected jobs:gpu, got %s", got)
	}
	if job.Status != StatusQueued {
		t.Fatalf("expected queued, got %s", job.Status)
	}
}

func TestLeaseSweeperRequeuesExpiredJob(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		JobType:              "cleaning",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-2",
		Payload:              map[string]any{"dataset_id": 1},
	})
	if err != nil {
		t.Fatalf("create job returned error: %v", err)
	}
	if err := repo.UpdateStatus(job.ID, StatusRunning); err != nil {
		t.Fatalf("set running: %v", err)
	}
	if err := repo.SetLease(job.ID, time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("set lease: %v", err)
	}

	sw := NewSweeper(repo, pub, 3)
	if err := sw.Tick(time.Now()); err != nil {
		t.Fatalf("sweeper tick failed: %v", err)
	}

	got, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found after sweep", job.ID)
	}
	if got.Status != StatusQueued {
		t.Fatalf("expected queued after requeue, got %s", got.Status)
	}
	if got.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", got.RetryCount)
	}
	if pub.LastLane() != "jobs:cpu" {
		t.Fatalf("expected jobs:cpu requeue, got %s", pub.LastLane())
	}
}

func TestCreateZeroShotRequiresIdempotencyKey(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{"project_id":1,"dataset_id":1,"snapshot_id":1,"prompt":"person","required_resource_type":"gpu"}`))
	rec := httptest.NewRecorder()
	h.CreateZeroShot(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "idempotency_key") {
		t.Fatalf("expected idempotency_key error, got %s", rec.Body.String())
	}
}

func TestCreateZeroShotPersistsDatasetAndSnapshot(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-dataset-snapshot",
		"required_resource_type":"gpu"
	}`))
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/1", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", "1")
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, routeCtx))
	getRec := httptest.NewRecorder()
	h.GetJob(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"dataset_id":42`) {
		t.Fatalf("expected dataset_id in job response, got %s", getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"snapshot_id":9`) {
		t.Fatalf("expected snapshot_id in job response, got %s", getRec.Body.String())
	}
}

func TestCreateJobAppendsQueuedEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-queued-event",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 || events[0].EventType != "queued" {
		t.Fatalf("expected queued event, got %+v", events)
	}
}

func TestCreateCleaningPersistsRequiredCapabilities(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/cleaning", strings.NewReader(`{
		"project_id":1,
		"dataset_id":7,
		"snapshot_id":3,
		"rules":{"dark_threshold":0.2},
		"idempotency_key":"idem-capabilities",
		"required_resource_type":"cpu",
		"required_capabilities":["rules_engine","image_stats"]
	}`))
	createRec := httptest.NewRecorder()
	h.CreateCleaning(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/1", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", "1")
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, routeCtx))
	getRec := httptest.NewRecorder()
	h.GetJob(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"required_capabilities":["rules_engine","image_stats"]`) {
		t.Fatalf("expected required capabilities in job response, got %s", getRec.Body.String())
	}
}

func TestRepositoryClaimSetsWorkerIDAndLease(t *testing.T) {
	repo := NewInMemoryRepository()
	job, _, err := repo.CreateOrGet(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		JobType:              "cleaning",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-claim",
		Payload:              map[string]any{"dataset_id": 1},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	claimed, err := repo.Claim(job.ID, "worker-a", time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claimed.WorkerID != "worker-a" {
		t.Fatalf("expected worker-a, got %s", claimed.WorkerID)
	}
	if claimed.LeaseUntil == nil {
		t.Fatal("expected lease timestamp to be set")
	}
}

func TestServiceReportHeartbeatSetsWorkerLease(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-heartbeat",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportHeartbeat", job.ID, "worker-a", 30)

	got, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatalf("job %d not found", job.ID)
	}
	if got.Status != StatusRunning {
		t.Fatalf("expected running after heartbeat, got %s", got.Status)
	}
	if got.WorkerID != "worker-a" {
		t.Fatalf("expected worker-a, got %s", got.WorkerID)
	}
	if got.LeaseUntil == nil {
		t.Fatal("expected lease timestamp to be set")
	}

	events := svc.ListEvents(job.ID)
	if len(events) == 0 || events[len(events)-1].EventType != "heartbeat" {
		t.Fatalf("expected heartbeat event, got %+v", events)
	}
}

func TestServiceReportProgressUpdatesCounters(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-progress",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportHeartbeat", job.ID, "worker-a", 30)
	callServiceErrorMethod(t, svc, "ReportProgress", job.ID, "worker-a", 10, 8, 2)

	got, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatalf("job %d not found", job.ID)
	}
	if got.TotalItems != 10 || got.SucceededItems != 8 || got.FailedItems != 2 {
		t.Fatalf("expected updated counters, got %+v", got)
	}

	events := svc.ListEvents(job.ID)
	if len(events) == 0 || events[len(events)-1].EventType != "progress" {
		t.Fatalf("expected progress event, got %+v", events)
	}
}

func TestServiceReportItemErrorAppendsItemFailureEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-item-error",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportItemError", job.ID, int64(7), "bad annotation line", map[string]any{"object_key": "train/a.jpg"})

	events := svc.ListEvents(job.ID)
	if len(events) == 0 {
		t.Fatalf("expected events for job %d", job.ID)
	}
	last := events[len(events)-1]
	if last.EventType != "item_failed" {
		t.Fatalf("expected item_failed event, got %+v", last)
	}
	if last.ItemID == nil || *last.ItemID != 7 {
		t.Fatalf("expected item id 7, got %+v", last.ItemID)
	}
	if last.Detail["object_key"] != "train/a.jpg" {
		t.Fatalf("expected object_key detail, got %+v", last.Detail)
	}
}

func TestServiceReportTerminalPersistsCounters(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-terminal",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportHeartbeat", job.ID, "worker-a", 30)
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-a", StatusSucceededWithErrors, 10, 8, 2)

	got, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatalf("job %d not found", job.ID)
	}
	if got.Status != StatusSucceededWithErrors {
		t.Fatalf("expected succeeded_with_errors, got %s", got.Status)
	}
	if got.TotalItems != 10 || got.SucceededItems != 8 || got.FailedItems != 2 {
		t.Fatalf("expected updated counters, got %+v", got)
	}
	if got.WorkerID != "worker-a" {
		t.Fatalf("expected worker-a, got %s", got.WorkerID)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
}

func callServiceErrorMethod(t *testing.T, svc *Service, name string, args ...any) {
	t.Helper()

	method := reflect.ValueOf(svc).MethodByName(name)
	if !method.IsValid() {
		t.Fatalf("%s method missing", name)
	}

	callArgs := make([]reflect.Value, 0, len(args))
	for _, arg := range args {
		callArgs = append(callArgs, reflect.ValueOf(arg))
	}

	results := method.Call(callArgs)
	if len(results) != 1 {
		t.Fatalf("%s returned %d values, want 1", name, len(results))
	}
	if err, ok := results[0].Interface().(error); ok && err != nil {
		t.Fatalf("%s returned error: %v", name, err)
	}
}
