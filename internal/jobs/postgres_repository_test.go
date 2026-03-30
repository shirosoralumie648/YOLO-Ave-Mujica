package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryRoundTripCreateClaimAndEvents(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	key := "integration-" + time.Now().UTC().Format("20060102150405.000000000")

	job, created, err := repo.CreateOrGet(1, "zero-shot", "gpu", key, map[string]any{"prompt": "person"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if !created {
		t.Fatal("expected first create to insert a new job")
	}

	duplicate, created, err := repo.CreateOrGet(1, "zero-shot", "gpu", key, map[string]any{"prompt": "person"})
	if err != nil {
		t.Fatalf("create duplicate job: %v", err)
	}
	if created {
		t.Fatal("expected duplicate create to reuse the existing job")
	}
	if duplicate.ID != job.ID {
		t.Fatalf("expected duplicate to resolve same job id, got %d want %d", duplicate.ID, job.ID)
	}

	event, err := repo.AppendEvent(job.ID, nil, "info", "queued", "job queued", map[string]any{"job_type": "zero-shot"})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	if event.EventType != "queued" {
		t.Fatalf("expected queued event, got %+v", event)
	}

	claimed, err := repo.Claim(job.ID, "worker-a", time.Now().UTC().Add(30*time.Second))
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claimed.Status != StatusRunning {
		t.Fatalf("expected running after claim, got %s", claimed.Status)
	}
	if claimed.WorkerID != "worker-a" {
		t.Fatalf("expected worker-a, got %s", claimed.WorkerID)
	}
	if claimed.LeaseUntil == nil {
		t.Fatal("expected lease timestamp to be set")
	}

	expiredLease := time.Now().UTC().Add(-1 * time.Second)
	if err := repo.TouchLease(job.ID, "worker-b", expiredLease); err != nil {
		t.Fatalf("touch expired lease: %v", err)
	}

	expiredJobs := repo.ListExpiredRunning(time.Now().UTC())
	found := false
	for _, expired := range expiredJobs {
		if expired.ID == job.ID {
			found = true
			if expired.WorkerID != "worker-b" {
				t.Fatalf("expected expired job worker-b, got %s", expired.WorkerID)
			}
		}
	}
	if !found {
		t.Fatalf("expected expired job %d in result set", job.ID)
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 || events[0].EventType != "queued" {
		t.Fatalf("expected queued event in persisted events, got %+v", events)
	}
}
