package overview

import (
	"testing"
	"time"

	"yolo-ave-mujica/internal/tasks"
)

type fakeTaskSource struct {
	items []tasks.Task
}

func (f fakeTaskSource) ListTasks(_ int64, _ tasks.ListTasksFilter) ([]tasks.Task, error) {
	return f.items, nil
}

type fakeReviewSource struct {
	pending int
}

func (f fakeReviewSource) PendingCandidateCount(_ int64) (int, error) {
	return f.pending, nil
}

type fakeJobSource struct {
	failed int
}

func (f fakeJobSource) FailedRecentJobCount(_ int64) (int, error) {
	return f.failed, nil
}

func TestServiceBuildOverviewIncludesLongestIdleAndBlockers(t *testing.T) {
	now := time.Now().UTC()
	svc := NewService(
		fakeTaskSource{
			items: []tasks.Task{
				{
					ID:             1,
					ProjectID:      1,
					Title:          "Fix missing masks",
					Status:         tasks.StatusBlocked,
					Assignee:       "annotator-1",
					BlockerReason:  "missing source images",
					LastActivityAt: now.Add(-5 * time.Hour),
				},
				{
					ID:             2,
					ProjectID:      1,
					Title:          "Label batch A",
					Status:         tasks.StatusQueued,
					Assignee:       "annotator-2",
					LastActivityAt: now.Add(-2 * time.Hour),
				},
			},
		},
		fakeReviewSource{
			pending: 5,
		},
		fakeJobSource{
			failed: 2,
		},
	)

	got, err := svc.BuildOverview(1)
	if err != nil {
		t.Fatalf("build overview: %v", err)
	}

	if got.OpenTaskCount != 2 {
		t.Fatalf("expected open_task_count=2, got %d", got.OpenTaskCount)
	}
	if got.BlockedTaskCount != 1 {
		t.Fatalf("expected blocked_task_count=1, got %d", got.BlockedTaskCount)
	}
	if got.ReviewBacklogCount != 5 {
		t.Fatalf("expected review_backlog_count=5, got %d", got.ReviewBacklogCount)
	}
	if got.FailedRecentJobs != 2 {
		t.Fatalf("expected failed_recent_jobs=2, got %d", got.FailedRecentJobs)
	}
	if len(got.Blockers) != 1 {
		t.Fatalf("expected 1 blocker card, got %d", len(got.Blockers))
	}
	if got.Blockers[0].TaskID != 1 || got.Blockers[0].Reason != "missing source images" {
		t.Fatalf("unexpected blocker card: %+v", got.Blockers[0])
	}
	if got.LongestIdleTask == nil || got.LongestIdleTask.ID != 1 {
		t.Fatalf("expected longest idle task id 1, got %+v", got.LongestIdleTask)
	}
}
