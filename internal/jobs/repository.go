package jobs

import (
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	CreateOrGet(projectID int64, jobType, requiredResourceType, key string, payload map[string]any) (*Job, bool, error)
	Get(id int64) (*Job, bool)
	UpdateStatus(id int64, to string) error
	Claim(id int64, workerID string, leaseUntil time.Time) (*Job, error)
	TouchLease(id int64, workerID string, leaseUntil time.Time) error
	ListExpiredRunning(now time.Time) []*Job
	IncrementRetryCount(id int64) error
	MarkRetryWaiting(id int64) error
	MarkFailed(id int64, code, msg string) error
	AppendEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) (Event, error)
	ListEvents(jobID int64) ([]Event, error)
}

type InMemoryRepository struct {
	mu        sync.Mutex
	nextID    int64
	nextEvent int64
	byKey     map[string]*Job
	byID      map[int64]*Job
	events    map[int64][]Event
}

// NewInMemoryRepository is a lightweight stand-in for the future DB-backed repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID:    1,
		nextEvent: 1,
		byKey:     make(map[string]*Job),
		byID:      make(map[int64]*Job),
		events:    make(map[int64][]Event),
	}
}

// idempotencyKey scopes deduplication by project + job type + user-provided key.
func idempotencyKey(projectID int64, jobType, key string) string {
	return fmt.Sprintf("%d:%s:%s", projectID, jobType, key)
}

// CreateOrGet returns an existing job when the idempotency tuple already exists.
// created=true means a new job row (in-memory object) was generated.
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

// UpdateStatus enforces legal transitions and stamps started/finished timestamps.
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

func (r *InMemoryRepository) Claim(id int64, workerID string, leaseUntil time.Time) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("job %d not found", id)
	}
	if job.Status == StatusQueued {
		now := time.Now().UTC()
		job.Status = StatusRunning
		job.StartedAt = &now
	}
	job.WorkerID = workerID
	job.LeaseUntil = &leaseUntil
	return job, nil
}

func (r *InMemoryRepository) TouchLease(id int64, workerID string, leaseUntil time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("job %d not found", id)
	}
	job.WorkerID = workerID
	job.LeaseUntil = &leaseUntil
	return nil
}

func (r *InMemoryRepository) SetLease(id int64, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("job %d not found", id)
	}
	job.LeaseUntil = &until
	return nil
}

func (r *InMemoryRepository) ListExpiredRunning(now time.Time) []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := []*Job{}
	for _, job := range r.byID {
		if job.Status != StatusRunning || job.LeaseUntil == nil {
			continue
		}
		if job.LeaseUntil.Before(now) {
			out = append(out, job)
		}
	}
	return out
}

func (r *InMemoryRepository) IncrementRetryCount(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("job %d not found", id)
	}
	job.RetryCount++
	return nil
}

func (r *InMemoryRepository) MarkRetryWaiting(id int64) error {
	return r.UpdateStatus(id, StatusRetryWaiting)
}

func (r *InMemoryRepository) MarkFailed(id int64, code, msg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("job %d not found", id)
	}
	if err := CanTransition(job.Status, StatusFailed); err != nil {
		return err
	}
	job.Status = StatusFailed
	now := time.Now().UTC()
	job.FinishedAt = &now
	job.ErrorCode = code
	job.ErrorMsg = msg
	return nil
}

// Get returns a job by id.
func (r *InMemoryRepository) Get(id int64) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	return job, ok
}

func (r *InMemoryRepository) AppendEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) (Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byID[jobID]; !ok {
		return Event{}, fmt.Errorf("job %d not found", jobID)
	}

	ev := Event{
		ID:         r.nextEvent,
		JobID:      jobID,
		ItemID:     itemID,
		EventLevel: level,
		EventType:  typ,
		Message:    message,
		Detail:     detail,
		TS:         time.Now().UTC(),
	}
	r.nextEvent++
	r.events[jobID] = append(r.events[jobID], ev)
	return ev, nil
}

func (r *InMemoryRepository) ListEvents(jobID int64) ([]Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	events := r.events[jobID]
	out := make([]Event, len(events))
	copy(out, events)
	return out, nil
}

var _ Repository = (*InMemoryRepository)(nil)
