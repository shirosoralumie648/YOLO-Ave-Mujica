package overview

import "time"

type IdleTask struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Assignee       string    `json:"assignee,omitempty"`
	Status         string    `json:"status"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type BlockerCard struct {
	Kind        string `json:"kind"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Href        string `json:"href"`
}

type ProjectOverview struct {
	ProjectID        int64         `json:"project_id"`
	OpenTaskCount    int           `json:"open_task_count"`
	ReviewBacklog    int           `json:"review_backlog"`
	FailedRecentJobs int           `json:"failed_recent_jobs"`
	LongestIdleTask  *IdleTask     `json:"longest_idle_task,omitempty"`
	Blockers         []BlockerCard `json:"blockers"`
}
