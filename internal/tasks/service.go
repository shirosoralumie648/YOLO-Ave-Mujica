package tasks

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService() *Service {
	return NewServiceWithRepository(nil)
}

func NewServiceWithRepository(repo Repository) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo}
}

func (s *Service) CreateTask(in CreateTaskInput) (Task, error) {
	if in.ProjectID <= 0 {
		return Task{}, errors.New("project_id must be > 0")
	}
	if in.DatasetID != nil && *in.DatasetID <= 0 {
		return Task{}, errors.New("dataset_id must be > 0")
	}
	if in.SnapshotID != nil && *in.SnapshotID <= 0 {
		return Task{}, errors.New("snapshot_id must be > 0")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return Task{}, errors.New("title is required")
	}
	if in.Status == "" {
		in.Status = StatusReady
	}
	if err := validateStatus(in.Status); err != nil {
		return Task{}, err
	}
	if in.Priority == "" {
		in.Priority = PriorityNormal
	}
	if err := validatePriority(in.Priority); err != nil {
		return Task{}, err
	}
	if in.LastActivityAt.IsZero() {
		in.LastActivityAt = time.Now().UTC()
	}
	return s.repo.CreateTask(context.Background(), in)
}

func (s *Service) ListProjectTasks(projectID int64) ([]Task, error) {
	if projectID <= 0 {
		return nil, errors.New("project_id must be > 0")
	}
	return s.repo.ListProjectTasks(context.Background(), projectID)
}

func (s *Service) GetTask(taskID int64) (Task, bool, error) {
	if taskID <= 0 {
		return Task{}, false, errors.New("task id must be > 0")
	}
	return s.repo.GetTask(context.Background(), taskID)
}
