package tasks

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Repository interface {
	CreateTask(ctx context.Context, in CreateTaskInput) (Task, error)
	ListProjectTasks(ctx context.Context, projectID int64) ([]Task, error)
	GetTask(ctx context.Context, taskID int64) (Task, bool, error)
}

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]Task
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID: 1,
		items:  make(map[int64]Task),
	}
}

func (r *InMemoryRepository) CreateTask(_ context.Context, in CreateTaskInput) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	task := Task{
		ID:             r.nextID,
		ProjectID:      in.ProjectID,
		DatasetID:      in.DatasetID,
		SnapshotID:     in.SnapshotID,
		Title:          in.Title,
		Description:    in.Description,
		Assignee:       in.Assignee,
		Status:         in.Status,
		Priority:       in.Priority,
		DueAt:          in.DueAt,
		LastActivityAt: in.LastActivityAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if task.LastActivityAt.IsZero() {
		task.LastActivityAt = now
	}
	r.items[task.ID] = task
	r.nextID++
	return task, nil
}

func (r *InMemoryRepository) ListProjectTasks(_ context.Context, projectID int64) ([]Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Task, 0)
	for _, item := range r.items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *InMemoryRepository) GetTask(_ context.Context, taskID int64) (Task, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[taskID]
	return item, ok, nil
}

func validateStatus(status string) error {
	switch status {
	case StatusReady, StatusInProgress, StatusSubmitted, StatusReviewing, StatusReworkRequired, StatusAccepted, StatusPublished, StatusClosed:
		return nil
	default:
		return fmt.Errorf("status %q is invalid", status)
	}
}

func validatePriority(priority string) error {
	switch priority {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return nil
	default:
		return fmt.Errorf("priority %q is invalid", priority)
	}
}

var _ Repository = (*InMemoryRepository)(nil)
