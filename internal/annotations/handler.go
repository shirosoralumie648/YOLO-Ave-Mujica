package annotations

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type workspaceService interface {
	GetWorkspace(ctx context.Context, taskID int64) (Workspace, error)
	SaveWorkspaceDraft(ctx context.Context, taskID int64, in WorkspaceDraftInput) (Workspace, error)
	SubmitWorkspace(ctx context.Context, taskID int64, in SubmitInput) (Workspace, error)
}

type Handler struct {
	svc workspaceService
}

func NewHandler(svc workspaceService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	workspace, err := h.svc.GetWorkspace(r.Context(), taskID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in WorkspaceDraftInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	workspace, err := h.svc.SaveWorkspaceDraft(r.Context(), taskID, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) SubmitWorkspace(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in SubmitInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	workspace, err := h.svc.SubmitWorkspace(r.Context(), taskID, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func parseTaskID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if isNotFoundError(err) {
		status = http.StatusNotFound
	}
	writeError(w, status, err)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
