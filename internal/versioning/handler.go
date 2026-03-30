package versioning

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type DiffRequest struct {
	Before       []Annotation `json:"before"`
	After        []Annotation `json:"after"`
	IOUThreshold float64      `json:"iou_threshold"`
}

func (h *Handler) DiffSnapshots(w http.ResponseWriter, r *http.Request) {
	var in DiffRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	out := h.svc.DiffSnapshots(in.Before, in.After, in.IOUThreshold)
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
