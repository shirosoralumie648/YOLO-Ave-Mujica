package overview

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"yolo-ave-mujica/internal/tasks"
)

type TaskSource interface {
	ListProjectTasks(ctx context.Context, projectID int64) ([]tasks.Task, error)
}

type MetricsSource interface {
	PendingReviewCount(projectID int64) (int, error)
	FailedJobCountSince(projectID int64, since time.Time) (int, error)
}

type Service struct {
	tasks   TaskSource
	metrics MetricsSource
	now     func() time.Time
}

func NewService(tasksSource TaskSource, metricsSource MetricsSource, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		tasks:   tasksSource,
		metrics: metricsSource,
		now:     now,
	}
}

func (s *Service) GetProjectOverview(projectID int64) (ProjectOverview, error) {
	if projectID <= 0 {
		return ProjectOverview{}, fmt.Errorf("project_id must be > 0")
	}

	items, err := s.tasks.ListProjectTasks(context.Background(), projectID)
	if err != nil {
		return ProjectOverview{}, err
	}

	reviewBacklog, err := s.metrics.PendingReviewCount(projectID)
	if err != nil {
		return ProjectOverview{}, err
	}
	failedRecentJobs, err := s.metrics.FailedJobCountSince(projectID, s.now().Add(-24*time.Hour))
	if err != nil {
		return ProjectOverview{}, err
	}

	out := ProjectOverview{
		ProjectID:        projectID,
		ReviewBacklog:    reviewBacklog,
		FailedRecentJobs: failedRecentJobs,
		Blockers:         make([]BlockerCard, 0, 3),
	}

	for _, item := range items {
		if item.Status != tasks.StatusClosed && item.Status != tasks.StatusPublished {
			out.OpenTaskCount++
		}

		if item.Status == tasks.StatusClosed || item.Status == tasks.StatusPublished {
			continue
		}
		if out.LongestIdleTask == nil || item.LastActivityAt.Before(out.LongestIdleTask.LastActivityAt) {
			out.LongestIdleTask = &IdleTask{
				ID:             item.ID,
				Title:          item.Title,
				Assignee:       item.Assignee,
				Status:         item.Status,
				LastActivityAt: item.LastActivityAt,
			}
		}
	}

	if reviewBacklog > 0 {
		out.Blockers = append(out.Blockers, BlockerCard{
			Kind:        "review_backlog",
			Severity:    "high",
			Title:       "Review queue backlog",
			Description: fmt.Sprintf("%d items are waiting for review", reviewBacklog),
			Href:        fmt.Sprintf("/review?project_id=%d", projectID),
		})
	}
	if failedRecentJobs > 0 {
		out.Blockers = append(out.Blockers, BlockerCard{
			Kind:        "failed_jobs",
			Severity:    "high",
			Title:       "Recent failed training or processing jobs",
			Description: fmt.Sprintf("%d recent jobs failed in the last 24 hours", failedRecentJobs),
			Href:        fmt.Sprintf("/training/runs?project_id=%d&status=failed", projectID),
		})
	}
	if out.LongestIdleTask != nil {
		idle := s.now().Sub(out.LongestIdleTask.LastActivityAt)
		if idle >= 24*time.Hour {
			out.Blockers = append(out.Blockers, BlockerCard{
				Kind:        "longest_idle_task",
				Severity:    "medium",
				Title:       "Longest idle task",
				Description: fmt.Sprintf("Task %d has been idle for %d hours", out.LongestIdleTask.ID, int(idle.Hours())),
				Href:        fmt.Sprintf("/tasks/%d", out.LongestIdleTask.ID),
			})
		}
	}

	return out, nil
}

type PostgresMetricsSource struct {
	pool *pgxpool.Pool
}

func NewPostgresMetricsSource(pool *pgxpool.Pool) *PostgresMetricsSource {
	return &PostgresMetricsSource{pool: pool}
}

func (s *PostgresMetricsSource) PendingReviewCount(projectID int64) (int, error) {
	var count int
	err := s.pool.QueryRow(context.Background(), `
		select count(*)
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		where d.project_id = $1 and c.review_status = 'pending'
	`, projectID).Scan(&count)
	return count, err
}

func (s *PostgresMetricsSource) FailedJobCountSince(projectID int64, since time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(context.Background(), `
		select count(*)
		from jobs
		where project_id = $1
		  and status in ('failed', 'succeeded_with_errors')
		  and created_at >= $2
	`, projectID, since).Scan(&count)
	return count, err
}

var _ MetricsSource = (*PostgresMetricsSource)(nil)
