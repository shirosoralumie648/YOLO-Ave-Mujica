package publish

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/server"
	"yolo-ave-mujica/internal/tasks"
)

func TestHandlerReviewApproveAndOwnerApprove(t *testing.T) {
	repo := NewInMemoryRepository()
	taskRepo := tasks.NewInMemoryRepository()
	svc := NewService(repo, tasks.NewService(taskRepo))
	handler := NewHandler(svc)
	httpServer := server.NewHTTPServerWithModules(server.Modules{
		Publish: server.PublishRoutes{
			ListCandidates:    handler.ListSuggestedCandidates,
			CreateBatch:       handler.CreateBatch,
			GetBatch:          handler.GetBatch,
			ReplaceBatchItems: handler.ReplaceBatchItems,
			ReviewApprove:     handler.ReviewApprove,
			OwnerApprove:      handler.OwnerApprove,
			AddBatchFeedback:  handler.AddBatchFeedback,
			AddItemFeedback:   handler.AddItemFeedback,
			GetWorkspace:      handler.GetWorkspace,
			GetRecord:         handler.GetRecord,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches", strings.NewReader(`{
		"project_id": 1,
		"snapshot_id": 15,
		"source": "suggested",
		"items": [{
			"candidate_id": 401,
			"task_id": 51,
			"dataset_id": 9,
			"snapshot_id": 15,
			"item_payload": {"task": {"id": 51}}
		}]
	}`))
	createRec := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create batch 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	reviewReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches/1/review-approve", strings.NewReader(`{"actor":"reviewer-1"}`))
	reviewRec := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("expected review approve 200, got %d body=%s", reviewRec.Code, reviewRec.Body.String())
	}

	ownerReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches/1/owner-approve", strings.NewReader(`{"actor":"owner-1"}`))
	ownerRec := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("expected owner approve 200, got %d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	if !strings.Contains(ownerRec.Body.String(), `"publish_record_id":`) {
		t.Fatalf("expected publish_record_id in response, got %s", ownerRec.Body.String())
	}
}
