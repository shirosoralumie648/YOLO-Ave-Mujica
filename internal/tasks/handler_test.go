package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/server"
)

type fakeRepository struct {
	items  []Task
	nextID int64
}

func (r *fakeRepository) CreateTask(_ context.Context, in CreateTaskInput) (Task, error) {
	r.nextID++
	task := Task{
		ID:             r.nextID,
		ProjectID:      in.ProjectID,
		DatasetID:      in.DatasetID,
		SnapshotID:     in.SnapshotID,
		Title:          in.Title,
		Description:    in.Description,
		Assignee:       in.Assignee,
		Status:         in.Status,
		Priority:       in.Priority,
		DueAt:          in.DueAt,
		LastActivityAt: in.LastActivityAt,
	}
	r.items = append(r.items, task)
	return task, nil
}

func (r *fakeRepository) ListProjectTasks(_ context.Context, projectID int64) ([]Task, error) {
	out := make([]Task, 0, len(r.items))
	for _, item := range r.items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fakeRepository) GetTask(_ context.Context, taskID int64) (Task, bool, error) {
	for _, item := range r.items {
		if item.ID == taskID {
			return item, true, nil
		}
	}
	return Task{}, false, nil
}

func TestCreateListAndGetTask(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewServiceWithRepository(repo)
	h := NewHandler(svc)

	srv := server.NewHTTPServerWithModules(server.Modules{
		Tasks: server.TaskRoutes{
			ListProjectTasks:  h.ListProjectTasks,
			CreateProjectTask: h.CreateProjectTask,
			GetTask:           h.GetTask,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/projects/1/tasks", strings.NewReader(`{
		"title":"Review queue backlog",
		"description":"Triaging the oldest pending work",
		"assignee":"reviewer-1",
		"priority":"high",
		"dataset_id":1,
		"snapshot_id":2
	}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created Task
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task: %v", err)
	}
	if created.ProjectID != 1 {
		t.Fatalf("expected project_id=1, got %+v", created)
	}
	if created.Status != StatusReady {
		t.Fatalf("expected default ready status, got %+v", created)
	}
	if created.Priority != PriorityHigh {
		t.Fatalf("expected high priority, got %+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/projects/1/tasks", nil)
	listRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"title":"Review queue backlog"`) {
		t.Fatalf("expected task title in list response, got %s", listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/1", nil)
	getRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"assignee":"reviewer-1"`) {
		t.Fatalf("expected assignee in detail response, got %s", getRec.Body.String())
	}
}

func TestCreateTaskRejectsMissingTitle(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewServiceWithRepository(repo)
	h := NewHandler(svc)

	srv := server.NewHTTPServerWithModules(server.Modules{
		Tasks: server.TaskRoutes{
			ListProjectTasks:  h.ListProjectTasks,
			CreateProjectTask: h.CreateProjectTask,
			GetTask:           h.GetTask,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/1/tasks", strings.NewReader(`{"assignee":"reviewer-1"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "title") {
		t.Fatalf("expected title validation error, got %s", rec.Body.String())
	}
}
