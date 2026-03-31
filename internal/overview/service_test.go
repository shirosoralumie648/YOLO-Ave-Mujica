package overview

import (
	"testing"
	"time"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/review"
	"yolo-ave-mujica/internal/tasks"
)

type fakeTaskSource struct {
	items []tasks.Task
}

func (f fakeTaskSource) ListTasks(_ int64, _ tasks.ListTasksFilter) ([]tasks.Task, error) {
	return f.items, nil
}

type fakeReviewSource struct {
	items []review.Candidate
}

func (f fakeReviewSource) ListCandidates() []review.Candidate {
	return f.items
}

type fakeJobSource struct {
	items []jobs.Job
}

func (f fakeJobSource) ListJobs(projectID int64) ([]jobs.Job, error) {
	out := make([]jobs.Job, 0, len(f.items))
	for _, item := range f.items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}

func TestServiceBuildOverviewIncludesLongestIdleAndBlockers(t *testing.T) {
	now := time.Now().UTC()
	svc := NewService(
		fakeTaskSource{
			items: []tasks.Task{
				{
					ID:             1,
					ProjectID:      1,
					Title:          "Label batch A",
					Status:         tasks.StatusInProgress,
					Assignee:       "annotator-1",
					LastActivityAt: now.Add(-2 * time.Hour),
				},
				{
					ID:             2,
					ProjectID:      1,
					Title:          "Fix missing masks",
					Status:         tasks.StatusBlocked,
					Assignee:       "annotator-2",
					BlockerReason:  "missing source images",
					LastActivityAt: now.Add(-5 * time.Hour),
				},
				{
					ID:             3,
					ProjectID:      1,
					Title:          "Done task",
					Status:         tasks.StatusDone,
					Assignee:       "annotator-3",
					LastActivityAt: now.Add(-8 * time.Hour),
				},
			},
		},
		fakeReviewSource{
			items: []review.Candidate{
				{ID: 10, ReviewStatus: "pending"},
				{ID: 11, ReviewStatus: "pending"},
			},
		},
		fakeJobSource{
			items: []jobs.Job{
				{ID: 20, ProjectID: 1, Status: jobs.StatusFailed},
				{ID: 21, ProjectID: 1, Status: jobs.StatusSucceeded},
				{ID: 22, ProjectID: 2, Status: jobs.StatusFailed},
			},
		},
	)

	got, err := svc.BuildOverview(1)
	if err != nil {
		t.Fatalf("build overview: %v", err)
	}

	if got.OpenTasks != 2 {
		t.Fatalf("expected open_tasks=2, got %d", got.OpenTasks)
	}
	if got.BlockedTasks != 1 {
		t.Fatalf("expected blocked_tasks=1, got %d", got.BlockedTasks)
	}
	if got.ReviewBacklog != 2 {
		t.Fatalf("expected review_backlog=2, got %d", got.ReviewBacklog)
	}
	if got.FailedRecentJobs != 1 {
		t.Fatalf("expected failed_recent_jobs=1, got %d", got.FailedRecentJobs)
	}
	if len(got.Blockers) != 1 {
		t.Fatalf("expected 1 blocker card, got %d", len(got.Blockers))
	}
	if got.Blockers[0].TaskID != 2 || got.Blockers[0].Reason != "missing source images" {
		t.Fatalf("unexpected blocker card: %+v", got.Blockers[0])
	}
	if got.LongestIdleTask == nil || got.LongestIdleTask.ID != 2 {
		t.Fatalf("expected longest idle task id 2, got %+v", got.LongestIdleTask)
	}
}
