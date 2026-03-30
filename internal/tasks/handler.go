package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CreateProjectTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req struct {
		Title       string     `json:"title"`
		Description string     `json:"description,omitempty"`
		Assignee    string     `json:"assignee,omitempty"`
		Status      string     `json:"status,omitempty"`
		Priority    string     `json:"priority,omitempty"`
		DatasetID   *int64     `json:"dataset_id,omitempty"`
		SnapshotID  *int64     `json:"snapshot_id,omitempty"`
		DueAt       *time.Time `json:"due_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	task, err := h.svc.CreateTask(CreateTaskInput{
		ProjectID:   projectID,
		DatasetID:   req.DatasetID,
		SnapshotID:  req.SnapshotID,
		Title:       req.Title,
		Description: req.Description,
		Assignee:    req.Assignee,
		Status:      req.Status,
		Priority:    req.Priority,
		DueAt:       req.DueAt,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) ListProjectTasks(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	items, err := h.svc.ListProjectTasks(projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	task, ok, err := h.svc.GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("task %d not found", taskID))
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
