package overview

import (
	"testing"
	"time"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/tasks"
)

type fakeTaskSource struct {
	items []tasks.Task
}

func (f fakeTaskSource) ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error) {
	return f.items, nil
}

type fakeReviewSource struct {
	count int
}

func (f fakeReviewSource) PendingCandidateCount(projectID int64) (int, error) {
	return f.count, nil
}

type fakeJobSource struct {
	items []jobs.Job
}

func (f fakeJobSource) ListRecentFailedJobs(projectID int64, limit int) ([]jobs.Job, error) {
	return f.items, nil
}

func TestServiceBuildOverviewAggregatesCardsAndBlockers(t *testing.T) {
	old := time.Now().UTC().Add(-4 * time.Hour)
	service := NewService(
		fakeTaskSource{items: []tasks.Task{
			{ID: 1, ProjectID: 1, Title: "Blocked review batch", Kind: tasks.KindReview, Status: tasks.StatusBlocked, BlockerReason: "schema mismatch", LastActivityAt: old},
			{ID: 2, ProjectID: 1, Title: "Queued annotation batch", Kind: tasks.KindAnnotation, Status: tasks.StatusQueued, LastActivityAt: time.Now().UTC()},
		}},
		fakeReviewSource{count: 3},
		fakeJobSource{items: []jobs.Job{
			{ID: 9, ProjectID: 1, JobType: "zero-shot", Status: jobs.StatusFailed, ErrorMsg: "provider unavailable"},
		}},
	)

	out, err := service.BuildOverview(1)
	if err != nil {
		t.Fatalf("build overview: %v", err)
	}
	if len(out.SummaryCards) != 4 {
		t.Fatalf("expected 4 summary cards, got %+v", out.SummaryCards)
	}
	if out.LongestIdleTask == nil || out.LongestIdleTask.ID != 1 {
		t.Fatalf("expected longest idle task id 1, got %+v", out.LongestIdleTask)
	}
	if len(out.Blockers) < 2 {
		t.Fatalf("expected blocked-task and review backlog blockers, got %+v", out.Blockers)
	}
	if len(out.RecentFailedJobs) != 1 || out.RecentFailedJobs[0].ID != 9 {
		t.Fatalf("expected one recent failed job, got %+v", out.RecentFailedJobs)
	}
}
