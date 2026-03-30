package tasks

import "time"

const (
	StatusReady          = "ready"
	StatusInProgress     = "in_progress"
	StatusSubmitted      = "submitted"
	StatusReviewing      = "reviewing"
	StatusReworkRequired = "rework_required"
	StatusAccepted       = "accepted"
	StatusPublished      = "published"
	StatusClosed         = "closed"
)

const (
	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

type Task struct {
	ID             int64      `json:"id"`
	ProjectID      int64      `json:"project_id"`
	DatasetID      *int64     `json:"dataset_id,omitempty"`
	SnapshotID     *int64     `json:"snapshot_id,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Assignee       string     `json:"assignee,omitempty"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateTaskInput struct {
	ProjectID      int64      `json:"project_id"`
	DatasetID      *int64     `json:"dataset_id,omitempty"`
	SnapshotID     *int64     `json:"snapshot_id,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Assignee       string     `json:"assignee,omitempty"`
	Status         string     `json:"status,omitempty"`
	Priority       string     `json:"priority,omitempty"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	LastActivityAt time.Time  `json:"-"`
}
