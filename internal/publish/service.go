package publish

import (
	"context"
	"fmt"
	"strings"

	"yolo-ave-mujica/internal/tasks"
)

type TaskCreator interface {
	CreateTask(ctx context.Context, in tasks.CreateTaskInput) (tasks.Task, error)
}

type Service struct {
	repo        Repository
	taskCreator TaskCreator
}

type ApprovalInput struct {
	Actor    string                `json:"actor"`
	Feedback []CreateFeedbackInput `json:"feedback,omitempty"`
}

type Workspace struct {
	Batch Batch `json:"batch"`
}

func NewService(repo Repository, taskCreator TaskCreator) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo, taskCreator: taskCreator}
}

func (s *Service) ListSuggestedCandidates(ctx context.Context, projectID int64) ([]SuggestedCandidate, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id must be > 0")
	}
	return s.repo.ListSuggestedCandidates(ctx, projectID)
}

func (s *Service) CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error) {
	if in.ProjectID <= 0 {
		return Batch{}, fmt.Errorf("project_id must be > 0")
	}
	if in.SnapshotID <= 0 {
		return Batch{}, fmt.Errorf("snapshot_id must be > 0")
	}
	in.Source = normalizeSource(in.Source)
	if !isValidSource(in.Source) {
		return Batch{}, fmt.Errorf("invalid source %q", in.Source)
	}
	if len(in.Items) == 0 {
		return Batch{}, fmt.Errorf("items are required")
	}
	for _, item := range in.Items {
		if err := validateBatchItem(in.SnapshotID, item); err != nil {
			return Batch{}, err
		}
	}
	return s.repo.CreateBatch(ctx, in)
}

func (s *Service) GetBatch(ctx context.Context, batchID int64) (Batch, error) {
	if batchID <= 0 {
		return Batch{}, fmt.Errorf("batch_id must be > 0")
	}
	return s.repo.GetBatch(ctx, batchID)
}

func (s *Service) GetRecord(ctx context.Context, recordID int64) (Record, error) {
	if recordID <= 0 {
		return Record{}, fmt.Errorf("record_id must be > 0")
	}
	return s.repo.GetRecord(ctx, recordID)
}

func (s *Service) GetWorkspace(ctx context.Context, batchID int64) (Workspace, error) {
	batch, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{Batch: batch}, nil
}

func (s *Service) ReplaceBatchItems(ctx context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error) {
	if batchID <= 0 {
		return Batch{}, fmt.Errorf("batch_id must be > 0")
	}
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return Batch{}, err
	}
	if !isEditableStatus(batch.Status) {
		return Batch{}, fmt.Errorf("batch %d is not editable", batchID)
	}
	in.Actor = normalizeActor(in.Actor)
	if len(in.Items) == 0 {
		return Batch{}, fmt.Errorf("items are required")
	}
	for _, item := range in.Items {
		if err := validateBatchItem(batch.SnapshotID, item); err != nil {
			return Batch{}, err
		}
	}
	return s.repo.ReplaceBatchItems(ctx, batchID, in)
}

func (s *Service) ReviewApprove(ctx context.Context, batchID int64, in ApprovalInput) error {
	return s.applyReviewDecision(ctx, batchID, ReviewDecisionApprove, in)
}

func (s *Service) ReviewReject(ctx context.Context, batchID int64, in ApprovalInput) error {
	return s.applyReviewDecision(ctx, batchID, ReviewDecisionReject, in)
}

func (s *Service) ReviewRework(ctx context.Context, batchID int64, in ApprovalInput) error {
	return s.applyReviewDecision(ctx, batchID, ReviewDecisionRework, in)
}

func (s *Service) OwnerApprove(ctx context.Context, batchID int64, in ApprovalInput) (Record, error) {
	if batchID <= 0 {
		return Record{}, fmt.Errorf("batch_id must be > 0")
	}
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return Record{}, err
	}
	if batch.Status != StatusReviewApproved && batch.Status != StatusOwnerPending {
		return Record{}, fmt.Errorf("batch %d is not ready for owner approval", batchID)
	}

	actor := normalizeActor(in.Actor)
	record, err := s.repo.ApplyOwnerDecision(ctx, batchID, OwnerDecisionApprove, actor, normalizeDecisionFeedback(in.Feedback, actor, FeedbackStageOwner))
	if err != nil {
		return Record{}, err
	}
	if s.taskCreator != nil {
		snapshotID := batch.SnapshotID
		if _, err := s.taskCreator.CreateTask(ctx, tasks.CreateTaskInput{
			ProjectID:  batch.ProjectID,
			SnapshotID: &snapshotID,
			Title:      fmt.Sprintf("Evaluate publish record %d", record.ID),
			Kind:       tasks.KindTrainingCandidate,
			Status:     tasks.StatusQueued,
			Priority:   tasks.PriorityHigh,
			Assignee:   "ml-engineer",
		}); err != nil {
			return Record{}, err
		}
	}
	return record, nil
}

func (s *Service) OwnerReject(ctx context.Context, batchID int64, in ApprovalInput) error {
	return s.applyOwnerDecision(ctx, batchID, OwnerDecisionReject, in)
}

func (s *Service) OwnerRework(ctx context.Context, batchID int64, in ApprovalInput) error {
	return s.applyOwnerDecision(ctx, batchID, OwnerDecisionRework, in)
}

func (s *Service) AddBatchFeedback(ctx context.Context, batchID int64, in CreateFeedbackInput) (Feedback, error) {
	if batchID <= 0 {
		return Feedback{}, fmt.Errorf("batch_id must be > 0")
	}
	in.Actor = normalizeActor(in.Actor)
	return s.repo.AddBatchFeedback(ctx, batchID, in)
}

func (s *Service) AddItemFeedback(ctx context.Context, batchID, itemID int64, in CreateFeedbackInput) (Feedback, error) {
	if batchID <= 0 {
		return Feedback{}, fmt.Errorf("batch_id must be > 0")
	}
	if itemID <= 0 {
		return Feedback{}, fmt.Errorf("item_id must be > 0")
	}
	in.Actor = normalizeActor(in.Actor)
	if in.Scope == "" {
		in.Scope = FeedbackScopeItem
	}
	return s.repo.AddItemFeedback(ctx, batchID, itemID, in)
}

func (s *Service) applyReviewDecision(ctx context.Context, batchID int64, decision string, in ApprovalInput) error {
	if batchID <= 0 {
		return fmt.Errorf("batch_id must be > 0")
	}
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if batch.Status == StatusPublished || batch.Status == StatusRejected || batch.Status == StatusSuperseded {
		return fmt.Errorf("batch %d is not reviewable", batchID)
	}
	actor := normalizeActor(in.Actor)
	return s.repo.ApplyReviewDecision(ctx, batchID, decision, actor, normalizeDecisionFeedback(in.Feedback, actor, FeedbackStageReview))
}

func (s *Service) applyOwnerDecision(ctx context.Context, batchID int64, decision string, in ApprovalInput) error {
	if batchID <= 0 {
		return fmt.Errorf("batch_id must be > 0")
	}
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if batch.Status != StatusReviewApproved && batch.Status != StatusOwnerPending {
		return fmt.Errorf("batch %d is not ready for owner decision", batchID)
	}
	_, err = s.repo.ApplyOwnerDecision(ctx, batchID, decision, normalizeActor(in.Actor), normalizeDecisionFeedback(in.Feedback, normalizeActor(in.Actor), FeedbackStageOwner))
	return err
}

func normalizeDecisionFeedback(items []CreateFeedbackInput, actor, stage string) []CreateFeedbackInput {
	out := make([]CreateFeedbackInput, 0, len(items))
	for _, item := range items {
		item.Actor = normalizeActor(item.Actor)
		if item.Actor == "system" && actor != "" {
			item.Actor = actor
		}
		if item.Stage == "" {
			item.Stage = stage
		}
		out = append(out, item)
	}
	return out
}

func normalizeSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		return SourceManual
	}
	return source
}

func isValidSource(source string) bool {
	switch source {
	case SourceSuggested, SourceManual:
		return true
	default:
		return false
	}
}

func validateBatchItem(batchSnapshotID int64, item CreateBatchItemInput) error {
	if item.CandidateID <= 0 {
		return fmt.Errorf("candidate_id must be > 0")
	}
	if item.TaskID <= 0 {
		return fmt.Errorf("task_id must be > 0")
	}
	if item.DatasetID <= 0 {
		return fmt.Errorf("dataset_id must be > 0")
	}
	if item.SnapshotID <= 0 {
		return fmt.Errorf("snapshot_id must be > 0")
	}
	if item.SnapshotID != batchSnapshotID {
		return fmt.Errorf("item snapshot_id %d must match batch snapshot_id %d", item.SnapshotID, batchSnapshotID)
	}
	return nil
}

func isEditableStatus(status string) bool {
	switch status {
	case StatusDraft, StatusReviewPending, StatusReviewApproved, StatusOwnerPending, StatusOwnerChangesRequested:
		return true
	default:
		return false
	}
}

func normalizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "system"
	}
	return actor
}
