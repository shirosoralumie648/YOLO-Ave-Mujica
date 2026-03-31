package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandlerCreateListAndGetTask(t *testing.T) {
	svc := NewService(NewInMemoryRepository())
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/projects/1/tasks", strings.NewReader(`{
		"snapshot_id": 2,
		"title": "Label parking-lot batch",
		"assignee": "annotator-1"
	}`))
	createRouteCtx := chi.NewRouteContext()
	createRouteCtx.URLParams.Add("id", "1")
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), chi.RouteCtxKey, createRouteCtx))
	createRec := httptest.NewRecorder()
	h.CreateTask(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created Task
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("expected created id 1, got %d", created.ID)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/projects/1/tasks?assignee=annotator-1", nil)
	listRouteCtx := chi.NewRouteContext()
	listRouteCtx.URLParams.Add("id", "1")
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), chi.RouteCtxKey, listRouteCtx))
	listRec := httptest.NewRecorder()
	h.ListTasks(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "Label parking-lot batch") {
		t.Fatalf("expected task title in list response, got %s", listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/1", nil)
	getRouteCtx := chi.NewRouteContext()
	getRouteCtx.URLParams.Add("id", "1")
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, getRouteCtx))
	getRec := httptest.NewRecorder()
	h.GetTask(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Label parking-lot batch") {
		t.Fatalf("expected task title in get response, got %s", getRec.Body.String())
	}
}
