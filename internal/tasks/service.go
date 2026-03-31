package tasks

import (
	"context"
	"errors"
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
	if in.Title == "" {
		return Task{}, errors.New("title is required")
	}
	if in.Kind == "" {
		in.Kind = KindAnnotation
	}
	if in.Status == "" {
		in.Status = StatusQueued
	}
	if in.Priority == "" {
		in.Priority = PriorityNormal
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
