package annotations

import (
	"time"

	"yolo-ave-mujica/internal/tasks"
)

const (
	StateDraft     = "draft"
	StateSubmitted = "submitted"
)

type Annotation struct {
	ID              int64          `json:"id"`
	TaskID          int64          `json:"task_id"`
	SnapshotID      int64          `json:"snapshot_id"`
	AssetObjectKey  string         `json:"asset_object_key"`
	FrameIndex      *int           `json:"frame_index,omitempty"`
	OntologyVersion string         `json:"ontology_version"`
	State           string         `json:"state"`
	Revision        int64          `json:"revision"`
	Body            map[string]any `json:"body"`
	SubmittedBy     string         `json:"submitted_by"`
	SubmittedAt     *time.Time     `json:"submitted_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type Asset struct {
	DatasetID       int64  `json:"dataset_id"`
	DatasetName     string `json:"dataset_name"`
	SnapshotID      *int64 `json:"snapshot_id,omitempty"`
	SnapshotVersion string `json:"snapshot_version"`
	ObjectKey       string `json:"object_key"`
	FrameIndex      *int   `json:"frame_index,omitempty"`
}

type Workspace struct {
	Task  tasks.Task `json:"task"`
	Asset Asset      `json:"asset"`
	Draft Annotation `json:"draft"`
}

type WorkspaceDraftInput struct {
	Actor        string         `json:"actor"`
	BaseRevision int64          `json:"base_revision,omitempty"`
	Body         map[string]any `json:"body"`
}
