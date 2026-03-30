package jobs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateGPUJobPublishesToGPULane(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(1, "zero-shot", "gpu", "idem-1", map[string]any{"prompt": "person"})
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

	job, err := svc.CreateJob(1, "cleaning", "cpu", "idem-2", map[string]any{"dataset_id": 1})
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

func TestCreateJobAppendsQueuedEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(1, "zero-shot", "gpu", "idem-queued-event", map[string]any{"prompt": "person"})
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

func TestRepositoryClaimSetsWorkerIDAndLease(t *testing.T) {
	repo := NewInMemoryRepository()
	job, _, err := repo.CreateOrGet(1, "cleaning", "cpu", "idem-claim", map[string]any{"dataset_id": 1})
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
