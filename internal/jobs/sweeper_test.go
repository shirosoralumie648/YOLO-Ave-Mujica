package jobs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"yolo-ave-mujica/internal/observability"
)

func TestLeaseSweeperSchedulesRetryBackoffAndAppendsRecoveryEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewService(repo)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-requeue",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.Claim(job.ID, "worker-a", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	now := time.Now().UTC()
	sw := NewSweeper(repo, pub, 3).WithRetryBackoff(5*time.Second, 30*time.Second)
	if err := sw.Tick(now); err != nil {
		t.Fatalf("tick: %v", err)
	}

	got, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found after sweep", job.ID)
	}
	if got.Status != StatusRetryWaiting {
		t.Fatalf("expected retry_waiting, got %s", got.Status)
	}
	if got.LeaseUntil == nil || !got.LeaseUntil.Equal(now.Add(5*time.Second)) {
		t.Fatalf("expected retry deadline %s, got %v", now.Add(5*time.Second), got.LeaseUntil)
	}
	if len(pub.items) != 0 {
		t.Fatalf("expected no immediate requeue publish during backoff, got %+v", pub.items)
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 2 || events[len(events)-1].EventType != "lease_recovered" {
		t.Fatalf("expected lease_recovered event, got %+v", events)
	}
	if got := events[len(events)-1].Detail["worker_id"]; got != "worker-a" {
		t.Fatalf("expected recovery event worker_id worker-a, got %#v", got)
	}
	if got := events[len(events)-1].Detail["retry_backoff_seconds"]; got != 5.0 {
		t.Fatalf("expected retry_backoff_seconds=5, got %#v", got)
	}
	if got := events[len(events)-1].Detail["retry_classification"]; got != "transient" {
		t.Fatalf("expected transient retry classification, got %#v", got)
	}
}

func TestLeaseSweeperRecordsRecoveryMetric(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewService(repo)
	metrics := observability.NewMetrics()

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-requeue-metric",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.Claim(job.ID, "worker-a", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	sw := NewSweeperWithMetrics(repo, pub, 3, metrics).WithRetryBackoff(5*time.Second, 30*time.Second)
	if err := sw.Tick(time.Now()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `yolo_job_lease_recoveries_total{job_type="zero-shot"} 1`) {
		t.Fatalf("expected lease recovery metric, got:\n%s", rec.Body.String())
	}
}

func TestLeaseSweeperRequeuesRetryWaitingJobAfterBackoff(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewService(repo)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-retry-due",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.Claim(job.ID, "worker-a", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	now := time.Now().UTC()
	sw := NewSweeper(repo, pub, 3).WithRetryBackoff(5*time.Second, 30*time.Second)
	if err := sw.Tick(now); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	if err := sw.Tick(now.Add(5 * time.Second)); err != nil {
		t.Fatalf("requeue due retry: %v", err)
	}

	got, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found after retry queue", job.ID)
	}
	if got.Status != StatusQueued {
		t.Fatalf("expected queued after retry backoff elapsed, got %s", got.Status)
	}
	if got.LeaseUntil != nil {
		t.Fatalf("expected queued retry job to clear lease_until, got %v", got.LeaseUntil)
	}
	if pub.LastLane() != "jobs:gpu" {
		t.Fatalf("expected jobs:gpu requeue, got %s", pub.LastLane())
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 3 || events[len(events)-1].EventType != "retry_requeued" {
		t.Fatalf("expected retry_requeued event, got %+v", events)
	}
}

func TestLeaseSweeperMarksFatalRetryClassificationAsFailed(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewService(repo)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-retry-fatal",
		Payload: map[string]any{
			"prompt": "person",
			"retry_policy": map[string]any{
				"classification": "fatal",
			},
		},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.Claim(job.ID, "worker-a", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	sw := NewSweeper(repo, pub, 3).WithRetryBackoff(5*time.Second, 30*time.Second)
	if err := sw.Tick(time.Now().UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	got, ok := repo.Get(job.ID)
	if !ok {
		t.Fatalf("job %d not found after fatal retry classification", job.ID)
	}
	if got.Status != StatusFailed {
		t.Fatalf("expected failed status for fatal retry classification, got %s", got.Status)
	}
	if got.RetryCount != 0 {
		t.Fatalf("expected retry_count to remain 0 for fatal retry classification, got %d", got.RetryCount)
	}
	if len(pub.items) != 0 {
		t.Fatalf("expected no requeue publish for fatal retry classification, got %+v", pub.items)
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 2 || events[len(events)-1].EventType != "lease_timeout" {
		t.Fatalf("expected lease_timeout event, got %+v", events)
	}
	if got := events[len(events)-1].Detail["retry_classification"]; got != "fatal" {
		t.Fatalf("expected fatal retry classification detail, got %#v", got)
	}
}
