package overview

import "yolo-ave-mujica/internal/tasks"

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

type ProjectOverview struct {
	SummaryCards     []SummaryCard   `json:"summary_cards"`
	Blockers         []BlockerCard   `json:"blockers"`
	LongestIdleTask  *tasks.Task     `json:"longest_idle_task,omitempty"`
	RecentFailedJobs []FailedJobItem `json:"recent_failed_jobs"`
}
