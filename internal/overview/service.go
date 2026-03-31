package overview

import (
	"errors"
	"sort"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/review"
	"yolo-ave-mujica/internal/tasks"
)

type TaskSource interface {
	ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error)
}

type ReviewSource interface {
	ListCandidates() []review.Candidate
}

type JobSource interface {
	ListJobs(projectID int64) ([]jobs.Job, error)
}

type BlockerCard struct {
	TaskID   int64  `json:"task_id"`
	Title    string `json:"title"`
	Assignee string `json:"assignee"`
	Reason   string `json:"reason"`
}

type Overview struct {
	ProjectID        int64         `json:"project_id"`
	OpenTasks        int           `json:"open_tasks"`
	BlockedTasks     int           `json:"blocked_tasks"`
	ReviewBacklog    int           `json:"review_backlog"`
	FailedRecentJobs int           `json:"failed_recent_jobs"`
	Blockers         []BlockerCard `json:"blockers"`
	LongestIdleTask  *tasks.Task   `json:"longest_idle_task,omitempty"`
}

type Service struct {
	tasks   TaskSource
	reviews ReviewSource
	jobs    JobSource
}

func NewService(taskSource TaskSource, reviewSource ReviewSource, jobSource JobSource) *Service {
	return &Service{
		tasks:   taskSource,
		reviews: reviewSource,
		jobs:    jobSource,
	}
}

func (s *Service) BuildOverview(projectID int64) (Overview, error) {
	if projectID <= 0 {
		return Overview{}, errors.New("project id must be > 0")
	}

	out := Overview{
		ProjectID: projectID,
		Blockers:  []BlockerCard{},
	}

	if s.tasks != nil {
		items, err := s.tasks.ListTasks(projectID, tasks.ListTasksFilter{})
		if err != nil {
			return Overview{}, err
		}
		for _, item := range items {
			if item.Status != tasks.StatusDone {
				out.OpenTasks++
				if out.LongestIdleTask == nil || item.LastActivityAt.Before(out.LongestIdleTask.LastActivityAt) {
					taskCopy := item
					out.LongestIdleTask = &taskCopy
				}
			}
			if item.Status == tasks.StatusBlocked {
				out.BlockedTasks++
				out.Blockers = append(out.Blockers, BlockerCard{
					TaskID:   item.ID,
					Title:    item.Title,
					Assignee: item.Assignee,
					Reason:   item.BlockerReason,
				})
			}
		}
		sort.Slice(out.Blockers, func(i, j int) bool { return out.Blockers[i].TaskID < out.Blockers[j].TaskID })
	}

	if s.reviews != nil {
		candidates := s.reviews.ListCandidates()
		for _, candidate := range candidates {
			if candidate.ReviewStatus == "" || candidate.ReviewStatus == "pending" {
				out.ReviewBacklog++
			}
		}
	}

	if s.jobs != nil {
		jobItems, err := s.jobs.ListJobs(projectID)
		if err != nil {
			return Overview{}, err
		}
		for _, job := range jobItems {
			if job.Status == jobs.StatusFailed {
				out.FailedRecentJobs++
			}
		}
	}

	return out, nil
}
