package overview

import (
	"context"
	"testing"
	"time"

	"yolo-ave-mujica/internal/tasks"
)

type fakeTaskSource struct {
	items []tasks.Task
}

func (s *fakeTaskSource) ListProjectTasks(_ context.Context, projectID int64) ([]tasks.Task, error) {
	out := make([]tasks.Task, 0, len(s.items))
	for _, item := range s.items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}

type fakeMetricsSource struct {
	reviewBacklog  int
	failedRecent   int
	lastProjectID  int64
	lastLookbackAt time.Time
}

func (s *fakeMetricsSource) PendingReviewCount(projectID int64) (int, error) {
	s.lastProjectID = projectID
	return s.reviewBacklog, nil
}

func (s *fakeMetricsSource) FailedJobCountSince(projectID int64, since time.Time) (int, error) {
	s.lastProjectID = projectID
	s.lastLookbackAt = since
	return s.failedRecent, nil
}

func TestServiceComputesOverviewSummaryAndBlockers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	oldest := now.Add(-72 * time.Hour)
	recent := now.Add(-2 * time.Hour)

	taskSource := &fakeTaskSource{
		items: []tasks.Task{
			{ID: 1, ProjectID: 1, Title: "Oldest pending review handoff", Assignee: "reviewer-1", Status: tasks.StatusReady, LastActivityAt: oldest},
			{ID: 2, ProjectID: 1, Title: "Fresh in progress task", Assignee: "annotator-1", Status: tasks.StatusInProgress, LastActivityAt: recent},
			{ID: 3, ProjectID: 1, Title: "Already closed", Assignee: "annotator-2", Status: tasks.StatusClosed, LastActivityAt: oldest},
		},
	}
	metrics := &fakeMetricsSource{reviewBacklog: 8, failedRecent: 2}

	svc := NewService(taskSource, metrics, func() time.Time { return now })
	got, err := svc.GetProjectOverview(1)
	if err != nil {
		t.Fatalf("get project overview: %v", err)
	}

	if got.ProjectID != 1 {
		t.Fatalf("expected project 1, got %+v", got)
	}
	if got.OpenTaskCount != 2 {
		t.Fatalf("expected 2 open tasks, got %+v", got)
	}
	if got.ReviewBacklog != 8 {
		t.Fatalf("expected review backlog 8, got %+v", got)
	}
	if got.FailedRecentJobs != 2 {
		t.Fatalf("expected 2 failed jobs, got %+v", got)
	}
	if got.LongestIdleTask == nil || got.LongestIdleTask.ID != 1 {
		t.Fatalf("expected task 1 to be longest idle, got %+v", got.LongestIdleTask)
	}
	if len(got.Blockers) != 3 {
		t.Fatalf("expected 3 blocker cards, got %+v", got.Blockers)
	}
}

func TestServiceOmitsBlockersWhenProjectLooksHealthy(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	taskSource := &fakeTaskSource{
		items: []tasks.Task{
			{ID: 1, ProjectID: 1, Title: "Fresh accepted task", Status: tasks.StatusAccepted, LastActivityAt: now.Add(-2 * time.Hour)},
		},
	}
	metrics := &fakeMetricsSource{}

	svc := NewService(taskSource, metrics, func() time.Time { return now })
	got, err := svc.GetProjectOverview(1)
	if err != nil {
		t.Fatalf("get project overview: %v", err)
	}

	if got.OpenTaskCount != 1 {
		t.Fatalf("expected one open task, got %+v", got)
	}
	if got.LongestIdleTask == nil || got.LongestIdleTask.ID != 1 {
		t.Fatalf("expected longest idle task 1, got %+v", got.LongestIdleTask)
	}
	if len(got.Blockers) != 0 {
		t.Fatalf("expected no blockers, got %+v", got.Blockers)
	}
}
