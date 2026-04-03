package tasks

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	CreateTask(ctx context.Context, in CreateTaskInput) (Task, error)
	ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error)
	GetTask(ctx context.Context, taskID int64) (Task, error)
	TransitionTask(ctx context.Context, taskID int64, in TransitionTaskInput) (Task, error)
}

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]Task
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID: 1,
		byID:   make(map[int64]Task),
	}
}

func (r *InMemoryRepository) CreateTask(_ context.Context, in CreateTaskInput) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	lastActivityAt := in.LastActivityAt
	if lastActivityAt.IsZero() {
		lastActivityAt = now
	}
	task := Task{
		ID:              r.nextID,
		ProjectID:       in.ProjectID,
		SnapshotID:      in.SnapshotID,
		Title:           in.Title,
		Kind:            in.Kind,
		AssetObjectKey:  in.AssetObjectKey,
		MediaKind:       in.MediaKind,
		FrameIndex:      in.FrameIndex,
		OntologyVersion: in.OntologyVersion,
		Status:          in.Status,
		Priority:        in.Priority,
		Assignee:        in.Assignee,
		DueAt:           in.DueAt,
		BlockerReason:   in.BlockerReason,
		LastActivityAt:  lastActivityAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	r.nextID++
	r.byID[task.ID] = task
	return task, nil
}

func (r *InMemoryRepository) ListTasks(_ context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Task, 0, len(r.byID))
	for _, task := range r.byID {
		if task.ProjectID != projectID {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.Kind != "" && task.Kind != filter.Kind {
			continue
		}
		if filter.Assignee != "" && task.Assignee != filter.Assignee {
			continue
		}
		if filter.Priority != "" && task.Priority != filter.Priority {
			continue
		}
		if filter.SnapshotID != nil {
			if task.SnapshotID == nil || *task.SnapshotID != *filter.SnapshotID {
				continue
			}
		}
		out = append(out, task)
	}
	return out, nil
}

func (r *InMemoryRepository) GetTask(_ context.Context, taskID int64) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.byID[taskID]
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", taskID)
	}
	return task, nil
}

func (r *InMemoryRepository) TransitionTask(_ context.Context, taskID int64, in TransitionTaskInput) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.byID[taskID]
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", taskID)
	}

	task.Status = in.Status
	task.BlockerReason = in.BlockerReason
	task.LastActivityAt = in.LastActivityAt
	task.UpdatedAt = time.Now().UTC()
	r.byID[taskID] = task
	return task, nil
}

var _ Repository = (*InMemoryRepository)(nil)
