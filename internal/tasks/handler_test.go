package tasks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandlerCreateListAndGetTask(t *testing.T) {
	svc := NewService(NewInMemoryRepository())
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/projects/1/tasks", strings.NewReader(`{
		"title":"Label loading-dock batch",
		"kind":"annotation",
		"priority":"high",
		"assignee":"annotator-1"
	}`))
	createCtx := chi.NewRouteContext()
	createCtx.URLParams.Add("id", "1")
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), chi.RouteCtxKey, createCtx))
	createRec := httptest.NewRecorder()
	h.CreateTask(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/projects/1/tasks?assignee=annotator-1", nil)
	listCtx := chi.NewRouteContext()
	listCtx.URLParams.Add("id", "1")
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), chi.RouteCtxKey, listCtx))
	listRec := httptest.NewRecorder()
	h.ListTasks(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), `"Label loading-dock batch"`) {
		t.Fatalf("expected created task in list response, got %d %s", listRec.Code, listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/1", nil)
	getCtx := chi.NewRouteContext()
	getCtx.URLParams.Add("id", "1")
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, getCtx))
	getRec := httptest.NewRecorder()
	h.GetTask(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"annotator-1"`) {
		t.Fatalf("expected created task in get response, got %d %s", getRec.Code, getRec.Body.String())
	}
}

func TestHandlerTransitionsTask(t *testing.T) {
	svc := NewService(NewInMemoryRepository())
	h := NewHandler(svc)

	created, err := svc.CreateTask(context.Background(), CreateTaskInput{
		ProjectID: 1,
		Title:     "Transition me",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/1/transition", strings.NewReader(`{"status":"ready"}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", strconv.FormatInt(created.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	rec := httptest.NewRecorder()
	h.TransitionTask(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected transition response, got %d %s", rec.Code, rec.Body.String())
	}
}
