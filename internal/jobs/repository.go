package jobs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type Repository interface {
	CreateOrGet(in CreateJobInput) (*Job, bool, error)
	Get(id int64) (*Job, bool)
	UpdateStatus(id int64, to string) error
	Claim(id int64, workerID string, leaseUntil time.Time) (*Job, error)
	TouchLease(id int64, workerID string, leaseUntil time.Time) error
	UpdateProgress(id int64, workerID string, total, succeeded, failed int) error
	Complete(id int64, workerID, status string, total, succeeded, failed int) error
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
func idempotencyKey(in CreateJobInput) string {
	return fmt.Sprintf("%d:%s:%s", in.ProjectID, in.JobType, in.IdempotencyKey)
}

// CreateOrGet returns an existing job when the idempotency tuple already exists.
// created=true means a new job row (in-memory object) was generated.
func (r *InMemoryRepository) CreateOrGet(in CreateJobInput) (*Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := idempotencyKey(in)
	if existing, ok := r.byKey[idx]; ok {
		return existing, false, nil
	}

	now := time.Now().UTC()
	job := &Job{
		ID:                   r.nextID,
		ProjectID:            in.ProjectID,
		DatasetID:            in.DatasetID,
		SnapshotID:           in.SnapshotID,
		JobType:              in.JobType,
		Status:               StatusQueued,
		RequiredResourceType: in.RequiredResourceType,
		RequiredCapabilities: append([]string(nil), in.RequiredCapabilities...),
		IdempotencyKey:       in.IdempotencyKey,
		Payload:              in.Payload,
		CreatedAt:            now,
	}
	r.nextID++
	r.byKey[idx] = job
	r.byID[job.ID] = job
	return job, true, nil
}

func payloadInt64(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	switch v := payload[key].(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		parsed, _ := strconv.ParseInt(string(v), 10, 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(v, 10, 64)
		return parsed
	default:
		return 0
	}
}

// UpdateStatus enforces legal transitions and stamps started/finished timestamps.
func (r *InMemoryRepository) UpdateStatus(id int64, to string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return newNotFoundError("job %d not found", id)
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
		return nil, newNotFoundError("job %d not found", id)
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
		return newNotFoundError("job %d not found", id)
	}
	job.WorkerID = workerID
	job.LeaseUntil = &leaseUntil
	return nil
}

func (r *InMemoryRepository) UpdateProgress(id int64, workerID string, total, succeeded, failed int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return newNotFoundError("job %d not found", id)
	}
	if job.Status != StatusRunning {
		if err := CanTransition(job.Status, StatusRunning); err != nil {
			return err
		}
		now := time.Now().UTC()
		job.Status = StatusRunning
		job.StartedAt = &now
	}
	job.WorkerID = workerID
	job.TotalItems = total
	job.SucceededItems = succeeded
	job.FailedItems = failed
	return nil
}

func (r *InMemoryRepository) Complete(id int64, workerID, status string, total, succeeded, failed int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return newNotFoundError("job %d not found", id)
	}
	fromStatus := job.Status
	if fromStatus == StatusQueued || fromStatus == StatusRetryWaiting {
		fromStatus = StatusRunning
	}
	if err := CanTransition(fromStatus, status); err != nil {
		return err
	}
	now := time.Now().UTC()
	if job.StartedAt == nil {
		job.StartedAt = &now
	}
	job.Status = status
	job.WorkerID = workerID
	job.TotalItems = total
	job.SucceededItems = succeeded
	job.FailedItems = failed
	job.FinishedAt = &now
	return nil
}

func (r *InMemoryRepository) SetLease(id int64, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[id]
	if !ok {
		return newNotFoundError("job %d not found", id)
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
		return newNotFoundError("job %d not found", id)
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
		return newNotFoundError("job %d not found", id)
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
		return Event{}, newNotFoundError("job %d not found", jobID)
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
