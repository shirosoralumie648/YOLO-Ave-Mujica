package review

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type reviewActionRequest struct {
	ReviewerID string `json:"reviewer_id"`
	ReasonCode string `json:"reason_code,omitempty"`
}

func (h *Handler) ListCandidates(w http.ResponseWriter, r *http.Request) {
	items := h.svc.ListCandidates()
	if identity, ok := auth.IdentityFromContext(r.Context()); ok && len(identity.ProjectIDs) > 0 {
		filtered := make([]Candidate, 0, len(items))
		for _, item := range items {
			if identity.AllowsProject(item.ProjectID) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) AcceptCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in reviewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.ReviewerID == "" {
		in.ReviewerID = "system"
	}
	if candidate, ok := h.svc.GetCandidate(id); ok {
		if err := auth.RequireProjectAccess(r.Context(), candidate.ProjectID); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
	}

	if err := h.svc.AcceptCandidate(id, in.ReviewerID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) RejectCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in reviewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.ReviewerID == "" {
		in.ReviewerID = "system"
	}
	if candidate, ok := h.svc.GetCandidate(id); ok {
		if err := auth.RequireProjectAccess(r.Context(), candidate.ProjectID); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
	}

	if err := h.svc.RejectCandidate(id, in.ReviewerID, in.ReasonCode); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
