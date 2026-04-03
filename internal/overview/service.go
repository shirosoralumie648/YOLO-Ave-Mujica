package overview

import (
	"fmt"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/tasks"
)

type TaskSource interface {
	ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error)
}

type TaskSourceFunc func(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error)

func (f TaskSourceFunc) ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error) {
	return f(projectID, filter)
}

type ReviewSource interface {
	PendingCandidateCount(projectID int64) (int, error)
}

type ReviewSourceFunc func(projectID int64) (int, error)

func (f ReviewSourceFunc) PendingCandidateCount(projectID int64) (int, error) {
	return f(projectID)
}

type JobSource interface {
	ListRecentFailedJobs(projectID int64, limit int) ([]jobs.Job, error)
}

type JobSourceFunc func(projectID int64, limit int) ([]jobs.Job, error)

func (f JobSourceFunc) ListRecentFailedJobs(projectID int64, limit int) ([]jobs.Job, error) {
	return f(projectID, limit)
}

type SummaryCard struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
	Href  string `json:"href"`
}

type BlockerCard struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Href   string `json:"href"`
}

type FailedJobItem struct {
	ID       int64  `json:"id"`
	JobType  string `json:"job_type"`
	Status   string `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

type Response struct {
	SummaryCards   []SummaryCard   `json:"summary_cards"`
	Blockers       []BlockerCard   `json:"blockers"`
	LongestIdleTask *tasks.Task    `json:"longest_idle_task,omitempty"`
	RecentFailedJobs []FailedJobItem `json:"recent_failed_jobs"`
}

type Service struct {
	tasks  TaskSource
	review ReviewSource
	jobs   JobSource
}

func NewService(taskSource TaskSource, reviewSource ReviewSource, jobSource JobSource) *Service {
	return &Service{tasks: taskSource, review: reviewSource, jobs: jobSource}
}

func (s *Service) BuildOverview(projectID int64) (Response, error) {
	taskItems, err := s.tasks.ListTasks(projectID, tasks.ListTasksFilter{})
	if err != nil {
		return Response{}, err
	}
	reviewBacklog, err := s.review.PendingCandidateCount(projectID)
	if err != nil {
		return Response{}, err
	}
	failedJobs, err := s.jobs.ListRecentFailedJobs(projectID, 5)
	if err != nil {
		return Response{}, err
	}

	var (
		openCount    int
		blockedCount int
		longestIdle  *tasks.Task
		blockers     []BlockerCard
	)

	for i := range taskItems {
		task := taskItems[i]
		if task.Status != tasks.StatusDone {
			openCount++
		}
		if task.Status == tasks.StatusBlocked {
			blockedCount++
			blockers = append(blockers, BlockerCard{
				ID:     fmt.Sprintf("task-%d", task.ID),
				Title:  task.Title,
				Reason: task.BlockerReason,
				Href:   fmt.Sprintf("/tasks/%d", task.ID),
			})
		}
		if task.Status != tasks.StatusDone && (longestIdle == nil || task.LastActivityAt.Before(longestIdle.LastActivityAt)) {
			copyTask := task
			longestIdle = &copyTask
		}
	}

	if reviewBacklog > 0 {
		blockers = append(blockers, BlockerCard{
			ID:     "review-backlog",
			Title:  "Review backlog requires action",
			Reason: fmt.Sprintf("%d pending candidates remain in review", reviewBacklog),
			Href:   "/tasks?kind=review&status=ready",
		})
	}

	summaryCards := []SummaryCard{
		{ID: "open-tasks", Title: "Open Tasks", Count: openCount, Href: "/tasks"},
		{ID: "blocked-tasks", Title: "Blocked Tasks", Count: blockedCount, Href: "/tasks?status=blocked"},
		{ID: "review-backlog", Title: "Review Backlog", Count: reviewBacklog, Href: "/tasks?kind=review&status=ready"},
		{ID: "failed-jobs", Title: "Failed Jobs", Count: len(failedJobs), Href: "/tasks?status=blocked"},
	}

	recent := make([]FailedJobItem, 0, len(failedJobs))
	for _, job := range failedJobs {
		blockers = append(blockers, BlockerCard{
			ID:     fmt.Sprintf("job-%d", job.ID),
			Title:  fmt.Sprintf("Failed job: %s", job.JobType),
			Reason: job.ErrorMsg,
			Href:   "/tasks?status=blocked",
		})
		recent = append(recent, FailedJobItem{
			ID:       job.ID,
			JobType:  job.JobType,
			Status:   job.Status,
			ErrorMsg: job.ErrorMsg,
		})
	}

	return Response{
		SummaryCards:    summaryCards,
		Blockers:        blockers,
		LongestIdleTask: longestIdle,
		RecentFailedJobs: recent,
	}, nil
}
