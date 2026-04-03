package annotations

import "time"

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
