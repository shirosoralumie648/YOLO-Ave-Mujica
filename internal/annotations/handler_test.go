package annotations

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/tasks"
)

func TestHandlerGetWorkspaceReturnsTaskAndDraft(t *testing.T) {
	taskRepo := tasks.NewInMemoryRepository()
	taskSvc := tasks.NewService(taskRepo)
	snapshotID := int64(7)

	task, err := taskSvc.CreateTask(t.Context(), tasks.CreateTaskInput{
		ProjectID:       1,
		SnapshotID:      &snapshotID,
		Title:           "Annotate yard frame A",
		Kind:            tasks.KindAnnotation,
		Status:          tasks.StatusInProgress,
		AssetObjectKey:  "train/images/a.jpg",
		MediaKind:       tasks.MediaKindImage,
		OntologyVersion: "v1",
		Assignee:        "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	handler := NewHandler(NewServiceWithTaskService(NewInMemoryRepository(), taskSvc))
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+strconv.FormatInt(task.ID, 10)+"/workspace", nil)
	rec := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/v1/tasks/{id}/workspace", handler.GetWorkspace)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"asset_object_key\":\"train/images/a.jpg\"") {
		t.Fatalf("expected workspace asset context, got %s", rec.Body.String())
	}
}

func TestHandlerSaveDraftReturnsUpdatedWorkspace(t *testing.T) {
	taskRepo := tasks.NewInMemoryRepository()
	taskSvc := tasks.NewService(taskRepo)
	snapshotID := int64(8)

	task, err := taskSvc.CreateTask(t.Context(), tasks.CreateTaskInput{
		ProjectID:       1,
		SnapshotID:      &snapshotID,
		Title:           "Annotate yard frame B",
		Kind:            tasks.KindAnnotation,
		Status:          tasks.StatusInProgress,
		AssetObjectKey:  "train/images/b.jpg",
		MediaKind:       tasks.MediaKindImage,
		OntologyVersion: "v1",
		Assignee:        "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	handler := NewHandler(NewServiceWithTaskService(NewInMemoryRepository(), taskSvc))
	req := httptest.NewRequest(http.MethodPut, "/v1/tasks/"+strconv.FormatInt(task.ID, 10)+"/workspace/draft", strings.NewReader(`{"actor":"annotator-1","body":{"objects":[{"id":"box-1","label":"person"}]}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Put("/v1/tasks/{id}/workspace/draft", handler.SaveDraft)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"revision\":1") {
		t.Fatalf("expected revision 1 in response, got %s", rec.Body.String())
	}
}

func TestHandlerSubmitTransitionsTaskToSubmitted(t *testing.T) {
	taskRepo := tasks.NewInMemoryRepository()
	taskSvc := tasks.NewService(taskRepo)
	snapshotID := int64(9)

	task, err := taskSvc.CreateTask(t.Context(), tasks.CreateTaskInput{
		ProjectID:       1,
		SnapshotID:      &snapshotID,
		Title:           "Annotate yard frame C",
		Kind:            tasks.KindAnnotation,
		Status:          tasks.StatusInProgress,
		AssetObjectKey:  "train/images/c.jpg",
		MediaKind:       tasks.MediaKindImage,
		OntologyVersion: "v1",
		Assignee:        "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc := NewServiceWithTaskService(NewInMemoryRepository(), taskSvc)
	if _, err := svc.SaveWorkspaceDraft(t.Context(), task.ID, WorkspaceDraftInput{
		Actor: "annotator-1",
		Body:  map[string]any{"objects": []any{}},
	}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	handler := NewHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+strconv.FormatInt(task.ID, 10)+"/workspace/submit", strings.NewReader(`{"actor":"annotator-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/v1/tasks/{id}/workspace/submit", handler.SubmitWorkspace)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"status\":\"submitted\"") {
		t.Fatalf("expected submitted response, got %s", rec.Body.String())
	}
}
