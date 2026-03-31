package overview

import (
	"errors"
	"math"
	"sort"
	"time"

	"yolo-ave-mujica/internal/tasks"
)

type TaskSource interface {
	ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error)
}

type ReviewSource interface {
	PendingCandidateCount(projectID int64) (int, error)
}

type JobSource interface {
	FailedRecentJobCount(projectID int64) (int, error)
}

type BlockerCard struct {
	TaskID  int64  `json:"task_id"`
	Title   string `json:"title"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
	Minutes int64  `json:"minutes_idle"`
}

type Overview struct {
	OpenTaskCount      int           `json:"open_task_count"`
	BlockedTaskCount   int           `json:"blocked_task_count"`
	ReviewBacklogCount int           `json:"review_backlog_count"`
	FailedRecentJobs   int           `json:"failed_recent_jobs"`
	Blockers           []BlockerCard `json:"blockers"`
	LongestIdleTask    *tasks.Task   `json:"longest_idle_task,omitempty"`
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
		Blockers: []BlockerCard{},
	}

	if s.tasks != nil {
		items, err := s.tasks.ListTasks(projectID, tasks.ListTasksFilter{})
		if err != nil {
			return Overview{}, err
		}
		now := time.Now().UTC()
		for _, item := range items {
			if item.Status != tasks.StatusDone {
				out.OpenTaskCount++
				if out.LongestIdleTask == nil || item.LastActivityAt.Before(out.LongestIdleTask.LastActivityAt) {
					taskCopy := item
					out.LongestIdleTask = &taskCopy
				}
			}
			if item.Status == tasks.StatusBlocked {
				idleMinutes := int64(math.Max(0, now.Sub(item.LastActivityAt).Minutes()))
				out.BlockedTaskCount++
				out.Blockers = append(out.Blockers, BlockerCard{
					TaskID:  item.ID,
					Title:   item.Title,
					Reason:  item.BlockerReason,
					Status:  item.Status,
					Minutes: idleMinutes,
				})
			}
		}
		sort.Slice(out.Blockers, func(i, j int) bool { return out.Blockers[i].TaskID < out.Blockers[j].TaskID })
	}

	if s.reviews != nil {
		pending, err := s.reviews.PendingCandidateCount(projectID)
		if err != nil {
			return Overview{}, err
		}
		out.ReviewBacklogCount = pending
	}

	if s.jobs != nil {
		failed, err := s.jobs.FailedRecentJobCount(projectID)
		if err != nil {
			return Overview{}, err
		}
		out.FailedRecentJobs = failed
	}

	return out, nil
}
