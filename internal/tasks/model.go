package tasks

import "time"

const (
	KindAnnotation        = "annotation"
	KindReview            = "review"
	KindQA                = "qa"
	KindOps               = "ops"
	KindTrainingCandidate = "training_candidate"
	KindPromotionReview   = "promotion_review"
)

const (
	MediaKindImage = "image"
	MediaKindVideo = "video"
)

const (
	StatusQueued         = "queued"
	StatusReady          = "ready"
	StatusInProgress     = "in_progress"
	StatusBlocked        = "blocked"
	StatusSubmitted      = "submitted"
	StatusReviewing      = "reviewing"
	StatusReworkRequired = "rework_required"
	StatusAccepted       = "accepted"
	StatusPublished      = "published"
	StatusClosed         = "closed"
	StatusDone           = StatusClosed
)

const (
	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

type Task struct {
	ID              int64      `json:"id"`
	ProjectID       int64      `json:"project_id"`
	SnapshotID      *int64     `json:"snapshot_id,omitempty"`
	Title           string     `json:"title"`
	Kind            string     `json:"kind"`
	AssetObjectKey  string     `json:"asset_object_key"`
	MediaKind       string     `json:"media_kind"`
	FrameIndex      *int       `json:"frame_index,omitempty"`
	OntologyVersion string     `json:"ontology_version"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	Assignee        string     `json:"assignee"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	BlockerReason   string     `json:"blocker_reason"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	SnapshotVersion string `json:"snapshot_version"`
	DatasetID       int64  `json:"dataset_id"`
	DatasetName     string `json:"dataset_name"`
}

type CreateTaskInput struct {
	ProjectID       int64      `json:"project_id"`
	SnapshotID      *int64     `json:"snapshot_id,omitempty"`
	Title           string     `json:"title"`
	Kind            string     `json:"kind"`
	AssetObjectKey  string     `json:"asset_object_key"`
	MediaKind       string     `json:"media_kind"`
	FrameIndex      *int       `json:"frame_index,omitempty"`
	OntologyVersion string     `json:"ontology_version"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	Assignee        string     `json:"assignee"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	BlockerReason   string     `json:"blocker_reason"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
}

type TransitionTaskInput struct {
	Status         string    `json:"status"`
	BlockerReason  string    `json:"blocker_reason"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type ListTasksFilter struct {
	Status     string `json:"status"`
	Kind       string `json:"kind"`
	Assignee   string `json:"assignee"`
	Priority   string `json:"priority"`
	SnapshotID *int64 `json:"snapshot_id,omitempty"`
}
