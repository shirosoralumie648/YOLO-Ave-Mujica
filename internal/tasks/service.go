package tasks

import (
	"context"
	"errors"
	"fmt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo}
}

func (s *Service) CreateTask(in CreateTaskInput) (Task, error) {
	if in.ProjectID <= 0 {
		return Task{}, errors.New("project_id must be > 0")
	}
	if in.SnapshotID < 0 {
		return Task{}, errors.New("snapshot_id must be >= 0")
	}
	if in.Title == "" {
		return Task{}, errors.New("title is required")
	}
	if in.Kind == "" {
		in.Kind = KindAnnotation
	}
	if !isValidKind(in.Kind) {
		return Task{}, fmt.Errorf("kind must be one of: %s, %s, %s, %s", KindAnnotation, KindReview, KindQA, KindOps)
	}
	if in.Status == "" {
		in.Status = StatusQueued
	}
	if !isValidStatus(in.Status) {
		return Task{}, fmt.Errorf("status must be one of: %s, %s, %s, %s, %s", StatusQueued, StatusReady, StatusInProgress, StatusBlocked, StatusDone)
	}
	if in.Priority == "" {
		in.Priority = PriorityNormal
	}
	if !isValidPriority(in.Priority) {
		return Task{}, fmt.Errorf("priority must be one of: %s, %s, %s, %s", PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical)
	}
	return s.repo.CreateTask(context.Background(), in)
}

func (s *Service) ListTasks(projectID int64, filter ListTasksFilter) ([]Task, error) {
	if projectID <= 0 {
		return nil, errors.New("project_id must be > 0")
	}
	return s.repo.ListTasks(context.Background(), projectID, filter)
}

func (s *Service) GetTask(taskID int64) (Task, error) {
	if taskID <= 0 {
		return Task{}, errors.New("task id must be > 0")
	}
	return s.repo.GetTask(context.Background(), taskID)
}

func isValidKind(kind string) bool {
	switch kind {
	case KindAnnotation, KindReview, KindQA, KindOps:
		return true
	default:
		return false
	}
}

func isValidStatus(status string) bool {
	switch status {
	case StatusQueued, StatusReady, StatusInProgress, StatusBlocked, StatusDone:
		return true
	default:
		return false
	}
}

func isValidPriority(priority string) bool {
	switch priority {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return true
	default:
		return false
	}
}
