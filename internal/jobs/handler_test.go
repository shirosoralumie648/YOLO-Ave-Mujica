package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/audit"
	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/observability"
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

func TestLeaseSweeperTransitionsExpiredJobToRetryWaiting(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewService(repo)

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

	now := time.Now().UTC()
	sw := NewSweeper(repo, pub, 3).WithRetryBackoff(5*time.Second, 30*time.Second)
	if err := sw.Tick(now); err != nil {
		t.Fatalf("sweeper tick failed: %v", err)
	}

	got, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found after sweep", job.ID)
	}
	if got.Status != StatusRetryWaiting {
		t.Fatalf("expected retry_waiting after lease recovery, got %s", got.Status)
	}
	if got.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", got.RetryCount)
	}
	if got.LeaseUntil == nil {
		t.Fatal("expected retry_waiting job to have retry deadline in lease_until")
	}
	if !got.LeaseUntil.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("expected lease_until to hold retry deadline %s, got %v", now.Add(5*time.Second), got.LeaseUntil)
	}
	if len(pub.items) != 0 {
		t.Fatalf("expected no immediate requeue publish during backoff, got %+v", pub.items)
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

func TestCreateZeroShotRejectsProjectOutsideCallerScope(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":2,
		"dataset_id":1,
		"snapshot_id":1,
		"prompt":"person",
		"idempotency_key":"idem-authz",
		"required_resource_type":"gpu"
	}`))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.NewIdentity("reviewer-1", []int64{1})))
	rec := httptest.NewRecorder()

	h.CreateZeroShot(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := repo.Get(1); ok {
		t.Fatal("did not expect forbidden zero-shot request to create a job")
	}
}

func TestCreateZeroShotRejectsPromptExceedingMaxLength(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	prompt := strings.Repeat("x", zeroShotPromptMaxChars+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":1,
		"snapshot_id":1,
		"prompt":"`+prompt+`",
		"idempotency_key":"idem-prompt-too-long",
		"required_resource_type":"gpu"
	}`))
	rec := httptest.NewRecorder()
	h.CreateZeroShot(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "prompt must be <=") {
		t.Fatalf("expected prompt length validation error, got %s", rec.Body.String())
	}
	if _, ok := repo.Get(1); ok {
		t.Fatal("did not expect oversized prompt request to create a job")
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

func TestGetJobReturnsNotFoundOutsideCallerScope(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            2,
		DatasetID:            42,
		SnapshotID:           9,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-job-read-authz",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/1", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, routeCtx))
	getReq = getReq.WithContext(auth.WithIdentity(getReq.Context(), auth.NewIdentity("reviewer-1", []int64{1})))
	getRec := httptest.NewRecorder()

	h.GetJob(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", getRec.Code, getRec.Body.String())
	}
}

func TestListEventsReturnsNotFoundOutsideCallerScope(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            2,
		DatasetID:            42,
		SnapshotID:           9,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-job-events-authz",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := svc.ReportEvent(job.ID, nil, "info", "queued", "queued", nil); err != nil {
		t.Fatalf("report event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/1/events", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.NewIdentity("reviewer-1", []int64{1})))
	rec := httptest.NewRecorder()

	h.ListEvents(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateZeroShotWritesAuditEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	recorder := audit.NewRecorder()
	h := NewHandlerWithAudit(svc, recorder)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-audit",
		"required_resource_type":"gpu"
	}`))
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %+v", events)
	}
	if events[0].Action != "job.create.zero-shot" || events[0].ResourceType != "job" || events[0].ResourceID != "1" {
		t.Fatalf("unexpected audit event: %+v", events[0])
	}
}

func TestCreateZeroShotPersistsTraceIDFromRequestHeader(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-trace",
		"required_resource_type":"gpu"
	}`))
	createReq.Header.Set(observability.TraceIDHeader, "trace-job-123")
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	job, ok := repo.Get(1)
	if !ok {
		t.Fatal("expected created job to be persisted")
	}
	if got := job.Payload["trace_id"]; got != "trace-job-123" {
		t.Fatalf("expected trace_id in payload, got %#v", got)
	}
}

func TestServiceMetricsRecordJobLifecycle(t *testing.T) {
	repo := NewInMemoryRepository()
	metrics := observability.NewMetrics()
	svc := NewServiceWithDependenciesAndMetrics(repo, nil, nil, metrics)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-metrics",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := svc.ReportHeartbeat(job.ID, "worker-a", 30); err != nil {
		t.Fatalf("report heartbeat: %v", err)
	}
	if err := svc.ReportTerminal(job.ID, "worker-a", StatusSucceeded, 1, 1, 0, nil); err != nil {
		t.Fatalf("report terminal: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, needle := range []string{
		`yolo_job_creations_total{job_type="zero-shot"} 1`,
		`yolo_job_completions_total{job_type="zero-shot",status="succeeded"} 1`,
		`yolo_job_duration_seconds_count{job_type="zero-shot",status="succeeded"} 1`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", needle, body)
		}
	}
}

func TestCreateZeroShotPersistsCommandProviderPayload(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-provider-payload",
		"required_resource_type":"gpu",
		"provider":{
			"type":"command",
			"argv":["python3","/opt/providers/zero-shot.py"]
		}
	}`))
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	job, ok := repo.Get(1)
	if !ok {
		t.Fatal("expected created job to be persisted")
	}
	provider, ok := job.Payload["provider"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider payload to be persisted, got %#v", job.Payload["provider"])
	}
	if provider["type"] != "command" {
		t.Fatalf("expected command provider type, got %#v", provider["type"])
	}
	argv, ok := provider["argv"].([]string)
	if !ok {
		t.Fatalf("expected argv to decode as []string, got %#v", provider["argv"])
	}
	if !reflect.DeepEqual([]string{"python3", "/opt/providers/zero-shot.py"}, argv) {
		t.Fatalf("unexpected provider argv: %#v", argv)
	}
}

func TestCreateZeroShotPersistsBuiltinProviderAndItemsPayload(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-builtin-provider",
		"required_resource_type":"gpu",
		"items":[
			{"item_id":101,"object_key":"train/a.jpg"},
			{"item_id":102,"object_key":"train/b.jpg"}
		],
		"provider":{
			"type":"builtin",
			"name":"grounding_dino_fake"
		}
	}`))
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	job, ok := repo.Get(1)
	if !ok {
		t.Fatal("expected created job to be persisted")
	}
	provider, ok := job.Payload["provider"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider payload to be persisted, got %#v", job.Payload["provider"])
	}
	if provider["type"] != "builtin" {
		t.Fatalf("expected builtin provider type, got %#v", provider["type"])
	}
	if provider["name"] != "grounding_dino_fake" {
		t.Fatalf("expected builtin provider name, got %#v", provider["name"])
	}
	items, ok := job.Payload["items"].([]map[string]any)
	if !ok {
		t.Fatalf("expected zero-shot items to be persisted, got %#v", job.Payload["items"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 zero-shot items, got %#v", items)
	}
	if items[0]["item_id"] != int64(101) {
		t.Fatalf("expected first item_id=101, got %#v", items[0]["item_id"])
	}
	if items[0]["object_key"] != "train/a.jpg" {
		t.Fatalf("expected first object_key=train/a.jpg, got %#v", items[0]["object_key"])
	}
}

func TestCreateZeroShotPersistsProviderTimeout(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-provider-timeout",
		"required_resource_type":"gpu",
		"provider":{
			"type":"command",
			"argv":["python3","/opt/providers/zero-shot.py"],
			"timeout_seconds":12.5
		}
	}`))
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	job, ok := repo.Get(1)
	if !ok {
		t.Fatal("expected created job to be persisted")
	}
	provider, ok := job.Payload["provider"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider payload to be persisted, got %#v", job.Payload["provider"])
	}
	if provider["timeout_seconds"] != 12.5 {
		t.Fatalf("expected timeout_seconds=12.5, got %#v", provider["timeout_seconds"])
	}
}

func TestGetJobIncludesPayloadProviderDetails(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/zero-shot", strings.NewReader(`{
		"project_id":1,
		"dataset_id":42,
		"snapshot_id":9,
		"prompt":"person",
		"idempotency_key":"idem-provider-get",
		"required_resource_type":"gpu",
		"provider":{
			"type":"command",
			"argv":["python3","/opt/providers/zero-shot.py"]
		}
	}`))
	createRec := httptest.NewRecorder()
	h.CreateZeroShot(createRec, createReq)
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
	if !strings.Contains(getRec.Body.String(), `"payload":{"dataset_id":42,"prompt":"person","provider":{"argv":["python3","/opt/providers/zero-shot.py"],"type":"command"},"snapshot_id":9}`) {
		t.Fatalf("expected payload provider details in job response, got %s", getRec.Body.String())
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

func TestRegisterWorkerUpsertsMetadataAndListsWorkers(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	registerReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/workers/register", strings.NewReader(`{
		"worker_id":"zero-shot-a",
		"resource_lane":"jobs:gpu",
		"job_types":["zero-shot"],
		"capabilities":["zero_shot_inference","grounding_dino"]
	}`))
	registerRec := httptest.NewRecorder()

	registerMethod := reflect.ValueOf(h).MethodByName("RegisterWorker")
	if !registerMethod.IsValid() {
		t.Fatal("expected handler to expose RegisterWorker")
	}
	registerMethod.Call([]reflect.Value{reflect.ValueOf(registerRec), reflect.ValueOf(registerReq)})

	if registerRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on register, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}
	for _, needle := range []string{
		`"worker_id":"zero-shot-a"`,
		`"resource_lane":"jobs:gpu"`,
		`"job_types":["zero-shot"]`,
		`"capabilities":["grounding_dino","zero_shot_inference"]`,
	} {
		if !strings.Contains(registerRec.Body.String(), needle) {
			t.Fatalf("expected register response to contain %q, got %s", needle, registerRec.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/workers", nil)
	listRec := httptest.NewRecorder()
	listMethod := reflect.ValueOf(h).MethodByName("ListWorkers")
	if !listMethod.IsValid() {
		t.Fatal("expected handler to expose ListWorkers")
	}
	listMethod.Call([]reflect.Value{reflect.ValueOf(listRec), reflect.ValueOf(listReq)})

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	for _, needle := range []string{
		`"worker_id":"zero-shot-a"`,
		`"resource_lane":"jobs:gpu"`,
		`"job_types":["zero-shot"]`,
		`"capabilities":["grounding_dino","zero_shot_inference"]`,
	} {
		if !strings.Contains(listRec.Body.String(), needle) {
			t.Fatalf("expected list response to contain %q, got %s", needle, listRec.Body.String())
		}
	}
}

func TestRegisterWorkerRejectsInvalidLane(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/workers/register", strings.NewReader(`{
		"worker_id":"zero-shot-a",
		"resource_lane":"gpu",
		"job_types":["zero-shot"],
		"capabilities":["zero_shot_inference"]
	}`))
	rec := httptest.NewRecorder()

	registerMethod := reflect.ValueOf(h).MethodByName("RegisterWorker")
	if !registerMethod.IsValid() {
		t.Fatal("expected handler to expose RegisterWorker")
	}
	registerMethod.Call([]reflect.Value{reflect.ValueOf(rec), reflect.ValueOf(req)})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "resource_lane") {
		t.Fatalf("expected resource_lane validation error, got %s", rec.Body.String())
	}
}

func TestCreateVideoExtractRejectsCommandProviderWithoutArgv(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/video-extract", strings.NewReader(`{
		"project_id":1,
		"dataset_id":2,
		"fps":2,
		"idempotency_key":"idem-video-provider-invalid",
		"required_resource_type":"cpu",
		"provider":{
			"type":"command",
			"argv":[]
		}
	}`))
	rec := httptest.NewRecorder()
	h.CreateVideoExtract(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "provider.argv") {
		t.Fatalf("expected provider argv validation error, got %s", rec.Body.String())
	}
}

func TestCreateVideoExtractRejectsNonPositiveProviderTimeout(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/video-extract", strings.NewReader(`{
		"project_id":1,
		"dataset_id":2,
		"fps":2,
		"idempotency_key":"idem-video-provider-timeout-invalid",
		"required_resource_type":"cpu",
		"provider":{
			"type":"command",
			"argv":["python3","/opt/providers/video.py"],
			"timeout_seconds":0
		}
	}`))
	rec := httptest.NewRecorder()
	h.CreateVideoExtract(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "provider.timeout_seconds") {
		t.Fatalf("expected provider timeout validation error, got %s", rec.Body.String())
	}
}

func TestCreateVideoExtractPersistsBuiltinProviderAndSourceContext(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/video-extract", strings.NewReader(`{
		"project_id":1,
		"dataset_id":2,
		"fps":2,
		"duration_ms":3000,
		"source_object_key":"clips/a.mp4",
		"frame_prefix":"clips/a",
		"idempotency_key":"idem-video-builtin-provider",
		"required_resource_type":"cpu",
		"provider":{
			"type":"builtin",
			"name":"video_decode_fake"
		}
	}`))
	rec := httptest.NewRecorder()
	h.CreateVideoExtract(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	job, ok := repo.Get(1)
	if !ok {
		t.Fatal("expected created job to be persisted")
	}
	provider, ok := job.Payload["provider"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider payload to be persisted, got %#v", job.Payload["provider"])
	}
	if provider["type"] != "builtin" {
		t.Fatalf("expected builtin provider type, got %#v", provider["type"])
	}
	if provider["name"] != "video_decode_fake" {
		t.Fatalf("expected builtin provider name, got %#v", provider["name"])
	}
	if job.Payload["source_object_key"] != "clips/a.mp4" {
		t.Fatalf("expected source_object_key to be persisted, got %#v", job.Payload["source_object_key"])
	}
	if job.Payload["frame_prefix"] != "clips/a" {
		t.Fatalf("expected frame_prefix to be persisted, got %#v", job.Payload["frame_prefix"])
	}
	if job.Payload["duration_ms"] != 3000 {
		t.Fatalf("expected duration_ms to be persisted, got %#v", job.Payload["duration_ms"])
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

func TestGetJobIncludesCandidateIDsInResultRef(t *testing.T) {
	repo := NewInMemoryRepository()
	reviewSvc := review.NewService()
	svc := NewServiceWithReviewSink(repo, nil, reviewSinkAdapter{svc: reviewSvc})
	h := NewHandler(svc)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           2,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-http-job-candidates",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportEvent", job.ID, (*int64)(nil), "info", "review_candidates_materialized", "persisted review candidates", map[string]any{
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
				"bbox": map[string]any{
					"x": 10.0,
					"y": 11.0,
					"w": 12.0,
					"h": 13.0,
				},
			},
		},
	})
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-a", StatusSucceeded, 1, 1, 0, map[string]any{
		"result_type":  "annotation_candidates",
		"result_count": 1,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+strconv.FormatInt(job.ID, 10), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	h.GetJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"candidate_ids":[1]`) {
		t.Fatalf("expected candidate_ids in result_ref, got %s", rec.Body.String())
	}
}

func TestGetJobIncludesFramesInResultRef(t *testing.T) {
	svc := NewService(NewInMemoryRepository())
	h := NewHandler(svc)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		JobType:              "video-extract",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-http-job-frames",
		Payload:              map[string]any{"fps": 2},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportEvent", job.ID, (*int64)(nil), "info", "video_frames_materialized", "persisted frame results", map[string]any{
		"result_type":  "video_frames",
		"result_count": 2,
		"frames": []any{
			map[string]any{"frame_index": 0, "timestamp_ms": 0, "object_key": "clips/a/frame-0000.jpg"},
			map[string]any{"frame_index": 6, "timestamp_ms": 3000, "object_key": "clips/a/frame-0006.jpg"},
		},
	})
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-video", StatusSucceeded, 2, 2, 0, map[string]any{
		"result_type":  "video_frames",
		"result_count": 2,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+strconv.FormatInt(job.ID, 10), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	h.GetJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"frames":[`) {
		t.Fatalf("expected frames in result_ref, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object_key":"clips/a/frame-0006.jpg"`) {
		t.Fatalf("expected frame object key in result_ref, got %s", rec.Body.String())
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

func TestReportHeartbeatReturnsNotFoundForUnknownJob(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/999/heartbeat", strings.NewReader(`{
		"worker_id":"worker-a",
		"lease_seconds":30
	}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	h.ReportHeartbeat(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("expected not found error, got %s", rec.Body.String())
	}
}

func TestReportHeartbeatReturnsUnprocessableEntityForMissingWorkerID(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-heartbeat-invalid-payload",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/1/heartbeat", strings.NewReader(`{
		"worker_id":"",
		"lease_seconds":30
	}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	h.ReportHeartbeat(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "worker_id is required") {
		t.Fatalf("expected worker_id validation error, got %s", rec.Body.String())
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

func TestReportTerminalReturnsConflictWhenJobAlreadyCompleted(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-terminal-conflict",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	callServiceErrorMethod(t, svc, "ReportHeartbeat", job.ID, "worker-a", 30)
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-a", StatusSucceeded, 1, 1, 0, map[string]any(nil))

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/1/complete", strings.NewReader(`{
		"worker_id":"worker-a",
		"status":"failed",
		"total_items":1,
		"succeeded_items":0,
		"failed_items":1
	}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	h.ReportTerminal(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid transition") {
		t.Fatalf("expected invalid transition error, got %s", rec.Body.String())
	}
}

func TestReportItemErrorReturnsUnprocessableEntityForMissingItemID(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-item-event-invalid",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/1/events", strings.NewReader(`{
		"event_type":"item_failed",
		"message":"bad annotation line"
	}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("job_id", strconv.FormatInt(job.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	h.ReportItemError(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "item_id must be > 0") {
		t.Fatalf("expected item_id validation error, got %s", rec.Body.String())
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
	if len(got.ResultRef) == 0 {
		t.Fatalf("expected result_ref to be present, got %+v", got)
	}
	candidateIDs, ok := got.ResultRef["candidate_ids"].([]int64)
	if !ok {
		t.Fatalf("expected candidate_ids in result_ref, got %+v", got.ResultRef)
	}
	if len(candidateIDs) != 1 || candidateIDs[0] != candidates[0].ID {
		t.Fatalf("expected persisted candidate id in result_ref, got %+v want %d", candidateIDs, candidates[0].ID)
	}

	stored, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found in repo", job.ID)
	}
	if stored.ResultType != "annotation_candidates" || stored.ResultCount != 1 {
		t.Fatalf("expected stored result metadata on job row, got %+v", stored)
	}
	if len(stored.ResultRef) == 0 {
		t.Fatalf("expected stored result_ref on job row, got %+v", stored)
	}
}

func TestServiceGetJobRetainsVideoFrameResultDetailsAfterTerminalEvent(t *testing.T) {
	svc := NewService(NewInMemoryRepository())

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		JobType:              "video-extract",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-video-results",
		Payload:              map[string]any{"fps": 2},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	frameDetail := map[string]any{
		"result_type":  "video_frames",
		"result_count": 2,
		"frames": []any{
			map[string]any{
				"frame_index":  0,
				"timestamp_ms": 0,
				"object_key":   "clips/a/frame-0000.jpg",
			},
			map[string]any{
				"frame_index":  6,
				"timestamp_ms": 3000,
				"object_key":   "clips/a/frame-0006.jpg",
			},
		},
	}

	callServiceErrorMethod(t, svc, "ReportEvent", job.ID, (*int64)(nil), "info", "video_frames_materialized", "persisted frame results", frameDetail)
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-video", StatusSucceeded, 2, 2, 0, map[string]any{
		"result_type":  "video_frames",
		"result_count": 2,
	})

	got, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatalf("job %d not found", job.ID)
	}
	if got.ResultType != "video_frames" || got.ResultCount != 2 {
		t.Fatalf("expected video result metadata, got %+v", got)
	}
	if len(got.ResultRef) == 0 {
		t.Fatalf("expected result_ref to be present, got %+v", got)
	}
	frames, ok := got.ResultRef["frames"].([]any)
	if !ok {
		t.Fatalf("expected frames in result_ref, got %+v", got.ResultRef)
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames in result_ref, got %+v", frames)
	}
}

func TestServiceGetJobRetainsCleaningReportAfterTerminalEvent(t *testing.T) {
	svc := NewService(NewInMemoryRepository())

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           2,
		JobType:              "cleaning",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-cleaning-results",
		Payload:              map[string]any{"rules": map[string]any{"dark_threshold": 0.3}},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	reportDetail := map[string]any{
		"result_type":  "cleaning_report",
		"result_count": 2,
		"report": map[string]any{
			"summary": map[string]any{
				"invalid_bbox":      1,
				"category_mismatch": 1,
				"too_dark":          1,
			},
			"issues": []any{
				map[string]any{"item_id": 1, "rule": "invalid_bbox"},
				map[string]any{"item_id": 1, "rule": "too_dark"},
				map[string]any{"item_id": 2, "rule": "category_mismatch"},
			},
			"removal_candidates": []any{float64(1), float64(2)},
		},
	}

	callServiceErrorMethod(t, svc, "ReportEvent", job.ID, (*int64)(nil), "info", "cleaning_report_materialized", "persisted cleaning report", reportDetail)
	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-cleaning", StatusSucceededWithErrors, 3, 1, 2, map[string]any{
		"result_type":  "cleaning_report",
		"result_count": 2,
	})

	got, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatalf("job %d not found", job.ID)
	}
	if got.ResultType != "cleaning_report" || got.ResultCount != 2 {
		t.Fatalf("expected cleaning result metadata, got %+v", got)
	}
	report, ok := got.ResultRef["report"].(map[string]any)
	if !ok {
		t.Fatalf("expected report in result_ref, got %+v", got.ResultRef)
	}
	if summary, ok := report["summary"].(map[string]any); !ok || int64Value(summary["invalid_bbox"]) != 1 {
		t.Fatalf("expected invalid_bbox summary in result_ref, got %+v", report)
	}
	removalCandidates, ok := report["removal_candidates"].([]any)
	if !ok || len(removalCandidates) != 2 {
		t.Fatalf("expected removal_candidates in result_ref, got %+v", report)
	}
}

func TestServiceReportTerminalPersistsArtifactResultRefOnJobRecord(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           2,
		JobType:              "artifact-package",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "idem-artifact-result-ref",
		Payload:              map[string]any{"artifact_id": 9, "format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	callServiceErrorMethod(t, svc, "ReportTerminal", job.ID, "worker-packager", StatusSucceeded, 3, 3, 0, map[string]any{
		"result_type":  "artifacts",
		"result_count": 1,
		"artifact_ids": []int64{9},
	})

	stored, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found in repo", job.ID)
	}
	if stored.ResultType != "artifacts" || stored.ResultCount != 1 {
		t.Fatalf("expected stored artifact result metadata, got %+v", stored)
	}
	if len(stored.ResultArtifactIDs) != 1 || stored.ResultArtifactIDs[0] != 9 {
		t.Fatalf("expected stored result artifact ids [9], got %+v", stored.ResultArtifactIDs)
	}
	artifactIDs, ok := stored.ResultRef["artifact_ids"].([]int64)
	if !ok {
		t.Fatalf("expected artifact_ids in stored result_ref, got %+v", stored.ResultRef)
	}
	if len(artifactIDs) != 1 || artifactIDs[0] != 9 {
		t.Fatalf("expected stored artifact_ids [9], got %+v", artifactIDs)
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
