package publish

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type InMemoryRepository struct {
	mu         sync.Mutex
	nextBatch  int64
	nextItem   int64
	nextFeed   int64
	nextRecord int64
	batches    map[int64]Batch
	records    map[int64]Record
	suggested  []SuggestedCandidate
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextBatch:  1,
		nextItem:   1,
		nextFeed:   1,
		nextRecord: 1,
		batches:    make(map[int64]Batch),
		records:    make(map[int64]Record),
		suggested:  []SuggestedCandidate{},
	}
}

func (r *InMemoryRepository) CreateBatch(_ context.Context, in CreateBatchInput) (Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	batch := Batch{
		ID:               r.nextBatch,
		ProjectID:        in.ProjectID,
		SnapshotID:       in.SnapshotID,
		Source:           in.Source,
		Status:           StatusDraft,
		RuleSummary:      cloneJSONMap(in.RuleSummary),
		OwnerEditVersion: 0,
		Feedback:         []Feedback{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.nextBatch++

	batch.Items = make([]BatchItem, 0, len(in.Items))
	for i, item := range in.Items {
		batch.Items = append(batch.Items, BatchItem{
			ID:             r.nextItem,
			PublishBatchID: batch.ID,
			CandidateID:    item.CandidateID,
			TaskID:         item.TaskID,
			DatasetID:      item.DatasetID,
			SnapshotID:     item.SnapshotID,
			ItemPayload:    cloneJSONMap(item.ItemPayload),
			Position:       i,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		r.nextItem++
	}

	r.batches[batch.ID] = cloneBatch(batch)
	return cloneBatch(batch), nil
}

func (r *InMemoryRepository) GetBatch(_ context.Context, batchID int64) (Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return Batch{}, fmt.Errorf("publish batch %d not found", batchID)
	}
	return cloneBatch(batch), nil
}

func (r *InMemoryRepository) GetRecord(_ context.Context, recordID int64) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.records[recordID]
	if !ok {
		return Record{}, fmt.Errorf("publish record %d not found", recordID)
	}
	return cloneRecord(record), nil
}

func (r *InMemoryRepository) BuildWorkspace(_ context.Context, batchID int64) (Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return Workspace{}, fmt.Errorf("publish batch %d not found", batchID)
	}
	return buildWorkspace(cloneBatch(batch)), nil
}

func (r *InMemoryRepository) ReplaceBatchItems(_ context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return Batch{}, fmt.Errorf("publish batch %d not found", batchID)
	}

	now := time.Now().UTC()
	items := make([]BatchItem, 0, len(in.Items))
	for i, item := range in.Items {
		items = append(items, BatchItem{
			ID:             r.nextItem,
			PublishBatchID: batch.ID,
			CandidateID:    item.CandidateID,
			TaskID:         item.TaskID,
			DatasetID:      item.DatasetID,
			SnapshotID:     item.SnapshotID,
			ItemPayload:    cloneJSONMap(item.ItemPayload),
			Position:       i,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		r.nextItem++
	}

	batch.Items = items
	batch.Status = StatusOwnerChangesRequested
	batch.OwnerEditVersion++
	batch.ReviewApprovedAt = nil
	batch.ReviewApprovedBy = ""
	batch.OwnerDecidedAt = nil
	batch.OwnerDecidedBy = ""
	batch.UpdatedAt = now
	r.batches[batchID] = cloneBatch(batch)
	return cloneBatch(batch), nil
}

func (r *InMemoryRepository) ApplyReviewDecision(_ context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return fmt.Errorf("publish batch %d not found", batchID)
	}

	for _, item := range feedback {
		batch.Feedback = append(batch.Feedback, r.newFeedbackLocked(batchID, nil, item))
	}

	now := time.Now().UTC()
	switch decision {
	case ReviewDecisionApprove:
		batch.Status = StatusReviewApproved
		batch.ReviewApprovedAt = timePtr(now)
		batch.ReviewApprovedBy = actor
	case ReviewDecisionReject:
		batch.Status = StatusRejected
		batch.ReviewApprovedAt = nil
		batch.ReviewApprovedBy = ""
	case ReviewDecisionRework:
		batch.Status = StatusOwnerChangesRequested
		batch.ReviewApprovedAt = nil
		batch.ReviewApprovedBy = ""
	default:
		return fmt.Errorf("unsupported review decision %q", decision)
	}
	batch.UpdatedAt = now
	r.batches[batchID] = cloneBatch(batch)
	return nil
}

func (r *InMemoryRepository) ApplyOwnerDecision(_ context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return Record{}, fmt.Errorf("publish batch %d not found", batchID)
	}

	for _, item := range feedback {
		batch.Feedback = append(batch.Feedback, r.newFeedbackLocked(batchID, nil, item))
	}

	now := time.Now().UTC()
	batch.OwnerDecidedAt = timePtr(now)
	batch.OwnerDecidedBy = actor
	batch.UpdatedAt = now

	switch decision {
	case OwnerDecisionApprove:
		record := Record{
			ID:              r.nextRecord,
			ProjectID:       batch.ProjectID,
			SnapshotID:      batch.SnapshotID,
			PublishBatchID:  batch.ID,
			Status:          StatusPublished,
			Summary:         map[string]any{"decision": OwnerDecisionApprove, "batch_id": batch.ID, "snapshot_id": batch.SnapshotID},
			ApprovedByOwner: actor,
			ApprovedAt:      now,
			CreatedAt:       now,
		}
		r.nextRecord++
		batch.Status = StatusPublished
		r.records[record.ID] = cloneRecord(record)
		r.batches[batchID] = cloneBatch(batch)
		return cloneRecord(record), nil
	case OwnerDecisionReject:
		batch.Status = StatusRejected
	case OwnerDecisionRework:
		batch.Status = StatusOwnerChangesRequested
	default:
		return Record{}, fmt.Errorf("unsupported owner decision %q", decision)
	}

	r.batches[batchID] = cloneBatch(batch)
	return Record{}, nil
}

func (r *InMemoryRepository) AddBatchFeedback(_ context.Context, batchID int64, in CreateFeedbackInput) (Feedback, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return Feedback{}, fmt.Errorf("publish batch %d not found", batchID)
	}

	feedback := r.newFeedbackLocked(batchID, nil, in)
	batch.Feedback = append(batch.Feedback, feedback)
	batch.UpdatedAt = feedback.CreatedAt
	r.batches[batchID] = cloneBatch(batch)
	return feedback, nil
}

func (r *InMemoryRepository) AddItemFeedback(_ context.Context, batchID, itemID int64, in CreateFeedbackInput) (Feedback, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	batch, ok := r.batches[batchID]
	if !ok {
		return Feedback{}, fmt.Errorf("publish batch %d not found", batchID)
	}

	found := false
	for _, item := range batch.Items {
		if item.ID == itemID {
			found = true
			break
		}
	}
	if !found {
		return Feedback{}, fmt.Errorf("publish batch item %d not found", itemID)
	}

	feedback := r.newFeedbackLocked(batchID, &itemID, in)
	batch.Feedback = append(batch.Feedback, feedback)
	batch.UpdatedAt = feedback.CreatedAt
	r.batches[batchID] = cloneBatch(batch)
	return feedback, nil
}

func (r *InMemoryRepository) ListSuggestedCandidates(_ context.Context, _ int64) ([]SuggestedCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]SuggestedCandidate, 0, len(r.suggested))
	for _, group := range r.suggested {
		out = append(out, cloneSuggestedCandidate(group))
	}
	return out, nil
}

func (r *InMemoryRepository) SeedSuggestedCandidates(items []SuggestedCandidate) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.suggested = make([]SuggestedCandidate, 0, len(items))
	for _, item := range items {
		r.suggested = append(r.suggested, cloneSuggestedCandidate(item))
	}
}

func (r *InMemoryRepository) newFeedbackLocked(batchID int64, itemID *int64, in CreateFeedbackInput) Feedback {
	now := time.Now().UTC()
	scope := in.Scope
	if scope == "" {
		scope = FeedbackScopeBatch
		if itemID != nil {
			scope = FeedbackScopeItem
		}
	}
	stage := in.Stage
	if stage == "" {
		stage = FeedbackStageReview
	}
	action := in.Action
	if action == "" {
		action = FeedbackActionComment
	}
	reasonCode := in.ReasonCode
	if reasonCode == "" {
		reasonCode = "unspecified"
	}
	severity := in.Severity
	if severity == "" {
		severity = "medium"
	}
	influenceWeight := in.InfluenceWeight
	if influenceWeight == 0 {
		influenceWeight = 1
	}

	feedback := Feedback{
		ID:              r.nextFeed,
		PublishBatchID:  batchID,
		Scope:           scope,
		Stage:           stage,
		Action:          action,
		ReasonCode:      reasonCode,
		Severity:        severity,
		InfluenceWeight: influenceWeight,
		Comment:         in.Comment,
		CreatedBy:       in.Actor,
		CreatedAt:       now,
	}
	if itemID != nil {
		feedback.PublishBatchItemID = *itemID
	}
	r.nextFeed++
	return feedback
}

func cloneBatch(in Batch) Batch {
	out := in
	out.RuleSummary = cloneJSONMap(in.RuleSummary)
	if in.ReviewApprovedAt != nil {
		value := *in.ReviewApprovedAt
		out.ReviewApprovedAt = &value
	}
	if in.OwnerDecidedAt != nil {
		value := *in.OwnerDecidedAt
		out.OwnerDecidedAt = &value
	}
	out.Items = make([]BatchItem, 0, len(in.Items))
	for _, item := range in.Items {
		out.Items = append(out.Items, BatchItem{
			ID:             item.ID,
			PublishBatchID: item.PublishBatchID,
			CandidateID:    item.CandidateID,
			TaskID:         item.TaskID,
			DatasetID:      item.DatasetID,
			SnapshotID:     item.SnapshotID,
			ItemPayload:    cloneJSONMap(item.ItemPayload),
			Position:       item.Position,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
		})
	}
	out.Feedback = make([]Feedback, 0, len(in.Feedback))
	for _, feedback := range in.Feedback {
		out.Feedback = append(out.Feedback, feedback)
	}
	return out
}

func cloneRecord(in Record) Record {
	out := in
	out.Summary = cloneJSONMap(in.Summary)
	return out
}

func cloneSuggestedCandidate(in SuggestedCandidate) SuggestedCandidate {
	out := in
	out.Summary = cloneJSONMap(in.Summary)
	out.Items = make([]SuggestedCandidateItem, 0, len(in.Items))
	for _, item := range in.Items {
		out.Items = append(out.Items, SuggestedCandidateItem{
			CandidateID: item.CandidateID,
			TaskID:      item.TaskID,
			DatasetID:   item.DatasetID,
			ItemPayload: cloneJSONMap(item.ItemPayload),
		})
	}
	return out
}

func cloneJSONMap(in map[string]any) map[string]any {
	raw, err := marshalJSONMap(in)
	if err != nil {
		return map[string]any{}
	}

	var out map[string]any
	if err := unmarshalJSONMap(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func timePtr(value time.Time) *time.Time {
	return &value
}

var _ Repository = (*InMemoryRepository)(nil)
