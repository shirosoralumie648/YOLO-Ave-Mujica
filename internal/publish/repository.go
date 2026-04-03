package publish

import "context"

type Repository interface {
	CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error)
	GetBatch(ctx context.Context, batchID int64) (Batch, error)
	GetRecord(ctx context.Context, recordID int64) (Record, error)
	ReplaceBatchItems(ctx context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error)
	ApplyReviewDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) error
	AddBatchFeedback(ctx context.Context, batchID int64, in CreateFeedbackInput) (Feedback, error)
	AddItemFeedback(ctx context.Context, batchID, itemID int64, in CreateFeedbackInput) (Feedback, error)
	ListSuggestedCandidates(ctx context.Context, projectID int64) ([]SuggestedCandidate, error)
}
