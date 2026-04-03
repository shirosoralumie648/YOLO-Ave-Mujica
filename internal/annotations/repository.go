package annotations

import "context"

type SaveDraftInput struct {
	TaskID          int64          `json:"task_id"`
	Actor           string         `json:"actor"`
	SnapshotID      int64          `json:"snapshot_id"`
	AssetObjectKey  string         `json:"asset_object_key"`
	FrameIndex      *int           `json:"frame_index,omitempty"`
	OntologyVersion string         `json:"ontology_version"`
	BaseRevision    int64          `json:"base_revision,omitempty"`
	Body            map[string]any `json:"body"`
}

type SubmitInput struct {
	TaskID int64  `json:"task_id"`
	Actor  string `json:"actor"`
}

type Repository interface {
	SaveDraft(ctx context.Context, in SaveDraftInput) (Annotation, error)
	Submit(ctx context.Context, taskID int64, actor string) (Annotation, error)
	GetByTaskID(ctx context.Context, taskID int64) (Annotation, error)
}
