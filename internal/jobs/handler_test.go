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
	"yolo-ave-mujica/internal/review"
)

type reviewSinkAdapter struct {
	svc *review.Service
}

func (a reviewSinkAdapter) PersistCandidates(jobID int64, items []ReviewCandidateInput) ([]PersistedReviewCandidate, error) {
	inputs := make([]review.PersistCandidateInput, 0, len(items))
	for _, item := range items {
		inputs = append(inputs, review.PersistCandidateInput{
			DatasetID:    item.DatasetID,
			SnapshotID:   item.SnapshotID,
			ItemID:       item.ItemID,
			ObjectKey:    item.ObjectKey,
			CategoryID:   item.CategoryID,
			CategoryName: item.CategoryName,
			BBox: review.CandidateBBox{
				X: item.BBox.X,
				Y: item.BBox.Y,
				W: item.BBox.W,
				H: item.BBox.H,
			},
			Confidence: item.Confidence,
			ModelName:  item.ModelName,
			IsPseudo:   item.IsPseudo,
		})
	}
	persisted, err := a.svc.PersistCandidates(jobID, inputs)
	if err != nil {
		return nil, err
	}
	out := make([]PersistedReviewCandidate, 0, len(persisted))
	for _, candidate := range persisted {
		out = append(out, PersistedReviewCandidate{ID: candidate.ID})
	}
	return out, nil
}

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

func TestCreateZeroShotRejectsIncompatibleResourceType(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":1,
		"snapshot_id":1,
		"prompt":"person",
		"idempotency_key":"idem-invalid-resource",
		"required_resource_type":"cpu"
	}`))
	rec := httptest.NewRecorder()
	h.CreateZeroShot(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "resource") {
		t.Fatalf("expected resource-type validation error, got %s", rec.Body.String())
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
	if !strings.Contains(getRec.Body.String(), `"required_capabilities":["zero_shot_inference"]`) {
		t.Fatalf("expected default zero-shot capability in job response, got %s", getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"resource_lane":"jobs:gpu"`) {
		t.Fatalf("expected resource lane in job response, got %s", getRec.Body.String())
	}
}

func TestCreateJobAppendsDispatchRequestedEvent(t *testing.T) {
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
	if len(events) == 0 || events[0].EventType != "dispatch_requested" {
		t.Fatalf("expected dispatch_requested event, got %+v", events)
	}
	if got := events[0].Detail["resource_lane"]; got != "jobs:gpu" {
		t.Fatalf("expected dispatch_requested event to include resource lane, got %#v", got)
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

func TestGetJobIncludesDispatchDiagnostics(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            7,
		SnapshotID:           3,
		JobType:              "cleaning",
		RequiredResourceType: "cpu",
		RequiredCapabilities: []string{"rules_engine", "image_stats"},
		IdempotencyKey:       "idem-diagnostics",
		Payload:              map[string]any{"dataset_id": 7},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	callServiceErrorMethod(t, svc, "ReportHeartbeat", job.ID, "worker-a", 45)
	if err := repo.IncrementRetryCount(job.ID); err != nil {
		t.Fatalf("increment retry_count: %v", err)
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
	if !strings.Contains(getRec.Body.String(), `"worker_id":"worker-a"`) {
		t.Fatalf("expected worker_id in job response, got %s", getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"retry_count":1`) {
		t.Fatalf("expected retry_count in job response, got %s", getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"lease_until"`) {
		t.Fatalf("expected lease_until in job response, got %s", getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"resource_lane":"jobs:cpu"`) {
		t.Fatalf("expected resource_lane in job response, got %s", getRec.Body.String())
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

func TestReportItemErrorAcceptsGenericDispatchEventWithoutItemID(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-dispatch-event",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/1/events", strings.NewReader(`{
		"event_level":"warn",
		"event_type":"dispatch_rejected",
		"message":"worker missing capability",
		"detail_json":{"reason":"missing_capabilities"}
	}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	h.ReportItemError(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	events := svc.ListEvents(job.ID)
	last := events[len(events)-1]
	if last.EventType != "dispatch_rejected" {
		t.Fatalf("expected dispatch_rejected event, got %+v", last)
	}
	if last.Detail["reason"] != "missing_capabilities" {
		t.Fatalf("expected missing_capabilities detail, got %+v", last.Detail)
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
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-a", StatusSucceededWithErrors, 10, 8, 2, map[string]any(nil))

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

func TestServiceReportEventPersistsReviewCandidatesAndGetJobExposesResultMetadata(t *testing.T) {
	repo := NewInMemoryRepository()
	reviewSvc := review.NewService()
	svc := NewServiceWithReviewSink(repo, nil, reviewSinkAdapter{svc: reviewSvc})

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           2,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-zero-shot-results",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	candidateDetail := map[string]any{
		"result_type":  "annotation_candidates",
		"result_count": 1,
		"candidates": []any{
			map[string]any{
				"dataset_id":    float64(1),
				"snapshot_id":   float64(2),
				"item_id":       float64(9),
				"object_key":    "images/9.jpg",
				"category_id":   float64(3),
				"category_name": "person",
				"confidence":    0.91,
				"model_name":    "grounding-dino-mvp",
				"is_pseudo":     true,
				"bbox": map[string]any{
					"x": 10.0,
					"y": 11.0,
					"w": 12.0,
					"h": 13.0,
				},
			},
		},
	}

	callServiceErrorMethod(t, svc, "ReportEvent", job.ID, (*int64)(nil), "info", "review_candidates_materialized", "persisted review candidates", candidateDetail)
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-a", StatusSucceeded, 1, 1, 0, map[string]any{
		"result_type":  "annotation_candidates",
		"result_count": 1,
	})

	candidates := reviewSvc.ListCandidates()
	if len(candidates) != 1 {
		t.Fatalf("expected 1 persisted candidate, got %+v", candidates)
	}
	if candidates[0].Status != review.CandidateStatusQueuedForReview {
		t.Fatalf("expected queued candidate status, got %+v", candidates[0])
	}
	if candidates[0].Source.ModelName != "grounding-dino-mvp" {
		t.Fatalf("expected source model to round-trip, got %+v", candidates[0].Source)
	}

	got, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatalf("job %d not found", job.ID)
	}
	if got.ResultType != "annotation_candidates" || got.ResultCount != 1 {
		t.Fatalf("expected terminal result metadata, got %+v", got)
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
