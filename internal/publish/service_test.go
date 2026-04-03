package publish

import (
	"context"
	"testing"

	"yolo-ave-mujica/internal/tasks"
)

func TestServiceOwnerApproveCreatesPublishRecordAndDownstreamTask(t *testing.T) {
	repo := NewInMemoryRepository()
	taskRepo := tasks.NewInMemoryRepository()
	svc := NewService(repo, tasks.NewService(taskRepo))

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 15,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: 401,
			TaskID:      51,
			DatasetID:   9,
			SnapshotID:  15,
			ItemPayload: map[string]any{"task": map[string]any{"id": 51}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := svc.ReviewApprove(context.Background(), batch.ID, ApprovalInput{Actor: "reviewer-1"}); err != nil {
		t.Fatalf("review approve: %v", err)
	}

	record, err := svc.OwnerApprove(context.Background(), batch.ID, ApprovalInput{Actor: "owner-1"})
	if err != nil {
		t.Fatalf("owner approve: %v", err)
	}
	if record.PublishBatchID != batch.ID {
		t.Fatalf("expected publish_batch_id=%d, got %d", batch.ID, record.PublishBatchID)
	}

	created, err := tasks.NewService(taskRepo).ListTasks(context.Background(), 1, tasks.ListTasksFilter{Kind: tasks.KindTrainingCandidate})
	if err != nil {
		t.Fatalf("list downstream tasks: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 downstream task, got %d", len(created))
	}
}

func TestServiceOwnerEditRequiresReviewApprovalAgain(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil)

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 16,
		Source:     SourceManual,
		Items: []CreateBatchItemInput{{
			CandidateID: 402,
			TaskID:      52,
			DatasetID:   9,
			SnapshotID:  16,
			ItemPayload: map[string]any{"task": map[string]any{"id": 52}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := svc.ReviewApprove(context.Background(), batch.ID, ApprovalInput{Actor: "reviewer-1"}); err != nil {
		t.Fatalf("review approve: %v", err)
	}
	if _, err := svc.ReplaceBatchItems(context.Background(), batch.ID, ReplaceBatchItemsInput{
		Actor: "owner-1",
		Items: []CreateBatchItemInput{{
			CandidateID: 403,
			TaskID:      53,
			DatasetID:   9,
			SnapshotID:  16,
			ItemPayload: map[string]any{"task": map[string]any{"id": 53}},
		}},
	}); err != nil {
		t.Fatalf("replace batch items: %v", err)
	}

	if _, err := svc.OwnerApprove(context.Background(), batch.ID, ApprovalInput{Actor: "owner-1"}); err == nil {
		t.Fatal("expected owner approve to fail before renewed reviewer approval")
	}
}
