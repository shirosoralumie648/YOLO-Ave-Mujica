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

func TestServiceBuildWorkspaceReturnsBatchItemsDiffAndFeedback(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil)

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 17,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: 501,
			TaskID:      61,
			DatasetID:   10,
			SnapshotID:  17,
			ItemPayload: map[string]any{
				"overlay": map[string]any{"boxes": []any{map[string]any{"label": "car"}}},
				"diff":    map[string]any{"added": 1, "updated": 0, "removed": 0},
			},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	workspace, err := svc.GetWorkspace(context.Background(), batch.ID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if len(workspace.Items) != 1 {
		t.Fatalf("expected 1 workspace item, got %d", len(workspace.Items))
	}
	if workspace.Items[0].Overlay == nil {
		t.Fatalf("expected overlay metadata, got %+v", workspace.Items[0])
	}
}

func TestServiceBatchAndItemFeedbackAreStoredSeparately(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil)

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 18,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: 601,
			TaskID:      71,
			DatasetID:   12,
			SnapshotID:  18,
			ItemPayload: map[string]any{"task": map[string]any{"id": 71}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	if _, err := svc.AddBatchFeedback(context.Background(), batch.ID, CreateFeedbackInput{
		Stage:           FeedbackStageReview,
		Action:          FeedbackActionRework,
		Scope:           FeedbackScopeBatch,
		ReasonCode:      "coverage_gap",
		Severity:        "high",
		InfluenceWeight: 1.0,
		Comment:         "Need wider sample coverage",
		Actor:           "reviewer-1",
	}); err != nil {
		t.Fatalf("add batch feedback: %v", err)
	}

	if _, err := svc.AddItemFeedback(context.Background(), batch.ID, batch.Items[0].ID, CreateFeedbackInput{
		Stage:           FeedbackStageOwner,
		Action:          FeedbackActionReject,
		Scope:           FeedbackScopeItem,
		ReasonCode:      "trajectory_break",
		Severity:        "critical",
		InfluenceWeight: 1.0,
		Comment:         "Track continuity is broken",
		Actor:           "owner-1",
	}); err != nil {
		t.Fatalf("add item feedback: %v", err)
	}

	got, err := svc.GetBatch(context.Background(), batch.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if len(got.Feedback) != 2 {
		t.Fatalf("expected 2 feedback rows, got %d", len(got.Feedback))
	}
}
