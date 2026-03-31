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
}

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]Task
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID: 1,
		items:  map[int64]Task{},
	}
}

func (r *InMemoryRepository) CreateTask(_ context.Context, in CreateTaskInput) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	task := Task{
		ID:             r.nextID,
		ProjectID:      in.ProjectID,
		Title:          in.Title,
		Kind:           in.Kind,
		Status:         in.Status,
		Priority:       in.Priority,
		Assignee:       in.Assignee,
		LastActivityAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if in.SnapshotID > 0 {
		snapshotID := in.SnapshotID
		task.SnapshotID = &snapshotID
	}

	r.items[task.ID] = task
	r.nextID++
	return task, nil
}

func (r *InMemoryRepository) ListTasks(_ context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := []Task{}
	for _, task := range r.items {
		if task.ProjectID != projectID {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.Assignee != "" && task.Assignee != filter.Assignee {
			continue
		}
		out = append(out, task)
	}
	return out, nil
}

func (r *InMemoryRepository) GetTask(_ context.Context, taskID int64) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.items[taskID]
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", taskID)
	}
	return task, nil
}

var _ Repository = (*InMemoryRepository)(nil)
