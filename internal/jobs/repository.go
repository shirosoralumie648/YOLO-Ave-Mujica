package jobs

import (
	"fmt"
	"sync"
	"time"
)

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	byKey  map[string]*Job
	byID   map[int64]*Job
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID: 1,
		byKey:  make(map[string]*Job),
		byID:   make(map[int64]*Job),
	}
}

func idempotencyKey(projectID int64, jobType, key string) string {
	return fmt.Sprintf("%d:%s:%s", projectID, jobType, key)
}

func (r *InMemoryRepository) CreateOrGet(projectID int64, jobType, requiredResourceType, key string, payload map[string]any) (*Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := idempotencyKey(projectID, jobType, key)
	if existing, ok := r.byKey[idx]; ok {
		return existing, false, nil
	}

	now := time.Now().UTC()
	job := &Job{
		ID:                   r.nextID,
		ProjectID:            projectID,
		JobType:              jobType,
		Status:               StatusQueued,
		RequiredResourceType: requiredResourceType,
		IdempotencyKey:       key,
		Payload:              payload,
		CreatedAt:            now,
	}
	r.nextID++
	r.byKey[idx] = job
	r.byID[job.ID] = job
	return job, true, nil
}

func (r *InMemoryRepository) UpdateStatus(id int64, to string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("job %d not found", id)
	}
	if err := CanTransition(job.Status, to); err != nil {
		return err
	}
	job.Status = to
	now := time.Now().UTC()
	if to == StatusRunning {
		job.StartedAt = &now
	}
	if to == StatusSucceeded || to == StatusSucceededWithErrors || to == StatusFailed || to == StatusCanceled {
		job.FinishedAt = &now
	}
	return nil
}
