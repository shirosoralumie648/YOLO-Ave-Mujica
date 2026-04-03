package publish

import (
	"context"
	"time"
)

type Workspace struct {
	Batch   Batch           `json:"batch"`
	Items   []WorkspaceItem `json:"items"`
	History []TimelineEntry `json:"history"`
}

type WorkspaceItem struct {
	ItemID      int64          `json:"item_id"`
	CandidateID int64          `json:"candidate_id"`
	TaskID      int64          `json:"task_id"`
	Overlay     map[string]any `json:"overlay"`
	Diff        map[string]any `json:"diff"`
	Context     map[string]any `json:"context"`
	Feedback    []Feedback     `json:"feedback"`
}

type TimelineEntry struct {
	Stage  string    `json:"stage"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	At     time.Time `json:"at"`
}

type Repository interface {
	CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error)
	GetBatch(ctx context.Context, batchID int64) (Batch, error)
	GetRecord(ctx context.Context, recordID int64) (Record, error)
	BuildWorkspace(ctx context.Context, batchID int64) (Workspace, error)
	ReplaceBatchItems(ctx context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error)
	ApplyReviewDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) error
	ApplyOwnerDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) (Record, error)
	AddBatchFeedback(ctx context.Context, batchID int64, in CreateFeedbackInput) (Feedback, error)
	AddItemFeedback(ctx context.Context, batchID, itemID int64, in CreateFeedbackInput) (Feedback, error)
	ListSuggestedCandidates(ctx context.Context, projectID int64) ([]SuggestedCandidate, error)
}
