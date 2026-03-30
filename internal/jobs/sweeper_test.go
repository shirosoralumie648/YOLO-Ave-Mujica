package jobs

import (
	"testing"
	"time"
)

func TestLeaseSweeperRequeuesExpiredJobAndAppendsRecoveryEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(1, "zero-shot", "gpu", "idem-requeue", map[string]any{"prompt": "person"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.Claim(job.ID, "worker-a", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	sw := NewSweeper(repo, pub, 3)
	if err := sw.Tick(time.Now()); err != nil {
		t.Fatalf("tick: %v", err)
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
}
