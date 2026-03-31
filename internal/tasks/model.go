package tasks

import "time"

const (
	KindAnnotation = "annotation"
	KindReview     = "review"
	KindQA         = "qa"
	KindOps        = "ops"

	StatusQueued     = "queued"
	StatusReady      = "ready"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusDone       = "done"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

type Task struct {
	ID             int64      `json:"id"`
	ProjectID      int64      `json:"project_id"`
	SnapshotID     *int64     `json:"snapshot_id,omitempty"`
	Title          string     `json:"title"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	Assignee       string     `json:"assignee"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	BlockerReason  string     `json:"blocker_reason,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateTaskInput struct {
	ProjectID  int64  `json:"project_id"`
	SnapshotID int64  `json:"snapshot_id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	Assignee   string `json:"assignee"`
}

type ListTasksFilter struct {
	Status   string
	Assignee string
}
