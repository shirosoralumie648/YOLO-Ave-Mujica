package publish

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ListSuggestedCandidates(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	items, err := h.svc.ListSuggestedCandidates(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var in CreateBatchInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	batch, err := h.svc.CreateBatch(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, batch)
}

func (h *Handler) GetBatch(w http.ResponseWriter, r *http.Request) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	batch, err := h.svc.GetBatch(r.Context(), batchID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (h *Handler) ReplaceBatchItems(w http.ResponseWriter, r *http.Request) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in ReplaceBatchItemsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	batch, err := h.svc.ReplaceBatchItems(r.Context(), batchID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (h *Handler) ReviewApprove(w http.ResponseWriter, r *http.Request) {
	h.handleReviewDecision(w, r, h.svc.ReviewApprove)
}

func (h *Handler) ReviewReject(w http.ResponseWriter, r *http.Request) {
	h.handleReviewDecision(w, r, h.svc.ReviewReject)
}

func (h *Handler) ReviewRework(w http.ResponseWriter, r *http.Request) {
	h.handleReviewDecision(w, r, h.svc.ReviewRework)
}

func (h *Handler) OwnerApprove(w http.ResponseWriter, r *http.Request) {
	batchID, in, err := decodeApprovalInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	record, err := h.svc.OwnerApprove(r.Context(), batchID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"publish_record_id": record.ID,
		"record":            record,
	})
}

func (h *Handler) OwnerReject(w http.ResponseWriter, r *http.Request) {
	h.handleOwnerDecision(w, r, h.svc.OwnerReject)
}

func (h *Handler) OwnerRework(w http.ResponseWriter, r *http.Request) {
	h.handleOwnerDecision(w, r, h.svc.OwnerRework)
}

func (h *Handler) AddBatchFeedback(w http.ResponseWriter, r *http.Request) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in CreateFeedbackInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	feedback, err := h.svc.AddBatchFeedback(r.Context(), batchID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, feedback)
}

func (h *Handler) AddItemFeedback(w http.ResponseWriter, r *http.Request) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	itemID, err := parsePathID(r, "itemId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in CreateFeedbackInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	feedback, err := h.svc.AddItemFeedback(r.Context(), batchID, itemID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, feedback)
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	workspace, err := h.svc.GetWorkspace(r.Context(), batchID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) GetRecord(w http.ResponseWriter, r *http.Request) {
	recordID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	record, err := h.svc.GetRecord(r.Context(), recordID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleReviewDecision(w http.ResponseWriter, r *http.Request, fn func(context.Context, int64, ApprovalInput) error) {
	batchID, in, err := decodeApprovalInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := fn(r.Context(), batchID, in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) handleOwnerDecision(w http.ResponseWriter, r *http.Request, fn func(context.Context, int64, ApprovalInput) error) {
	batchID, in, err := decodeApprovalInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := fn(r.Context(), batchID, in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeApprovalInput(r *http.Request) (int64, ApprovalInput, error) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		return 0, ApprovalInput{}, err
	}

	var in ApprovalInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return 0, ApprovalInput{}, err
	}
	return batchID, in, nil
}

func parsePathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
