package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"
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

func (s *Service) CreateTask(ctx context.Context, in CreateTaskInput) (Task, error) {
	if in.ProjectID <= 0 {
		return Task{}, fmt.Errorf("project_id must be > 0")
	}
	if in.SnapshotID != nil && *in.SnapshotID <= 0 {
		return Task{}, fmt.Errorf("snapshot_id must be > 0 when provided")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	in.Kind = normalizeKind(in.Kind)
	in.Status = normalizeStatus(in.Status)
	in.Priority = normalizePriority(in.Priority)
	in.Assignee = strings.TrimSpace(in.Assignee)
	in.BlockerReason = strings.TrimSpace(in.BlockerReason)
	if in.LastActivityAt.IsZero() {
		in.LastActivityAt = time.Now().UTC()
	}

	if in.Status == StatusBlocked && in.BlockerReason == "" {
		return Task{}, fmt.Errorf("blocker_reason is required when status is blocked")
	}
	if !isValidKind(in.Kind) {
		return Task{}, fmt.Errorf("invalid kind %q", in.Kind)
	}
	if !isValidStatus(in.Status) {
		return Task{}, fmt.Errorf("invalid status %q", in.Status)
	}
	if !isValidPriority(in.Priority) {
		return Task{}, fmt.Errorf("invalid priority %q", in.Priority)
	}

	return s.repo.CreateTask(ctx, in)
}

func (s *Service) ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id must be > 0")
	}
	normalized, err := normalizeFilter(filter)
	if err != nil {
		return nil, err
	}
	return s.repo.ListTasks(ctx, projectID, normalized)
}

func (s *Service) GetTask(ctx context.Context, taskID int64) (Task, error) {
	if taskID <= 0 {
		return Task{}, fmt.Errorf("task_id must be > 0")
	}
	return s.repo.GetTask(ctx, taskID)
}

func (s *Service) TransitionTask(ctx context.Context, taskID int64, in TransitionTaskInput) (Task, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}

	in.Status = normalizeStatus(in.Status)
	in.BlockerReason = strings.TrimSpace(in.BlockerReason)
	if in.LastActivityAt.IsZero() {
		in.LastActivityAt = time.Now().UTC()
	}

	if !isValidStatus(in.Status) {
		return Task{}, fmt.Errorf("invalid status %q", in.Status)
	}
	if !isAllowedTransition(current.Status, in.Status) {
		return Task{}, fmt.Errorf("invalid status transition %q -> %q", current.Status, in.Status)
	}
	if in.Status == StatusBlocked && in.BlockerReason == "" {
		return Task{}, fmt.Errorf("blocker_reason is required when status is blocked")
	}
	if in.Status != StatusBlocked {
		in.BlockerReason = ""
	}

	return s.repo.TransitionTask(ctx, taskID, in)
}

func normalizeKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		return KindAnnotation
	}
	return kind
}

func normalizeStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		return StatusQueued
	}
	return status
}

func normalizePriority(priority string) string {
	priority = strings.TrimSpace(strings.ToLower(priority))
	if priority == "" {
		return PriorityNormal
	}
	return priority
}

func normalizeFilter(filter ListTasksFilter) (ListTasksFilter, error) {
	filter.Status = strings.TrimSpace(strings.ToLower(filter.Status))
	filter.Kind = strings.TrimSpace(strings.ToLower(filter.Kind))
	filter.Assignee = strings.TrimSpace(filter.Assignee)
	filter.Priority = strings.TrimSpace(strings.ToLower(filter.Priority))

	if filter.Status != "" && !isValidStatus(filter.Status) {
		return ListTasksFilter{}, fmt.Errorf("invalid status %q", filter.Status)
	}
	if filter.Kind != "" && !isValidKind(filter.Kind) {
		return ListTasksFilter{}, fmt.Errorf("invalid kind %q", filter.Kind)
	}
	if filter.Priority != "" && !isValidPriority(filter.Priority) {
		return ListTasksFilter{}, fmt.Errorf("invalid priority %q", filter.Priority)
	}
	if filter.SnapshotID != nil && *filter.SnapshotID <= 0 {
		return ListTasksFilter{}, fmt.Errorf("snapshot_id must be > 0 when provided")
	}

	return filter, nil
}

func isValidKind(kind string) bool {
	switch kind {
	case KindAnnotation, KindReview, KindQA, KindOps, KindTrainingCandidate, KindPromotionReview:
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

var allowedTransitions = map[string]map[string]bool{
	StatusQueued: {
		StatusReady: true,
	},
	StatusReady: {
		StatusInProgress: true,
		StatusBlocked:    true,
	},
	StatusInProgress: {
		StatusBlocked: true,
		StatusDone:    true,
	},
	StatusBlocked: {
		StatusReady:      true,
		StatusInProgress: true,
	},
}

func isAllowedTransition(from, to string) bool {
	return allowedTransitions[from][to]
}
