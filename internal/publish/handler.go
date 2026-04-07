package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/audit"
)

type Handler struct {
	svc   *Service
	audit audit.Logger
}

type feedbackRequest struct {
	Stage           string  `json:"stage"`
	Action          string  `json:"action"`
	ReasonCode      string  `json:"reason_code"`
	Severity        string  `json:"severity"`
	InfluenceWeight float64 `json:"influence_weight"`
	Comment         string  `json:"comment"`
	Actor           string  `json:"actor"`
}

func NewHandler(svc *Service) *Handler {
	return NewHandlerWithAudit(svc, nil)
}

func NewHandlerWithAudit(svc *Service, auditLogger audit.Logger) *Handler {
	return &Handler{svc: svc, audit: auditLogger}
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
	if err := h.recordAudit(r, audit.Event{
		Action:       "publish.batch.create",
		ResourceType: "publish_batch",
		ResourceID:   strconv.FormatInt(batch.ID, 10),
		Detail: map[string]any{
			"project_id":  batch.ProjectID,
			"snapshot_id": batch.SnapshotID,
			"source":      batch.Source,
			"items_count": len(batch.Items),
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	if err := h.recordAudit(r, audit.Event{
		Actor:        in.Actor,
		Action:       "publish.batch.replace-items",
		ResourceType: "publish_batch",
		ResourceID:   strconv.FormatInt(batch.ID, 10),
		Detail: map[string]any{
			"snapshot_id":        batch.SnapshotID,
			"owner_edit_version": batch.OwnerEditVersion,
			"items_count":        len(batch.Items),
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (h *Handler) ReviewApprove(w http.ResponseWriter, r *http.Request) {
	h.handleReviewDecision(w, r, "publish.batch.review-approve", h.svc.ReviewApprove)
}

func (h *Handler) ReviewReject(w http.ResponseWriter, r *http.Request) {
	h.handleReviewDecision(w, r, "publish.batch.review-reject", h.svc.ReviewReject)
}

func (h *Handler) ReviewRework(w http.ResponseWriter, r *http.Request) {
	h.handleReviewDecision(w, r, "publish.batch.review-rework", h.svc.ReviewRework)
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
	if err := h.recordAudit(r, audit.Event{
		Actor:        in.Actor,
		Action:       "publish.batch.owner-approve",
		ResourceType: "publish_batch",
		ResourceID:   strconv.FormatInt(batchID, 10),
		Detail: map[string]any{
			"publish_record_id": record.ID,
			"snapshot_id":       record.SnapshotID,
			"project_id":        record.ProjectID,
			"feedback_count":    len(in.Feedback),
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"publish_record_id": record.ID,
		"record":            record,
	})
}

func (h *Handler) OwnerReject(w http.ResponseWriter, r *http.Request) {
	h.handleOwnerDecision(w, r, "publish.batch.owner-reject", h.svc.OwnerReject)
}

func (h *Handler) OwnerRework(w http.ResponseWriter, r *http.Request) {
	h.handleOwnerDecision(w, r, "publish.batch.owner-rework", h.svc.OwnerRework)
}

func (h *Handler) AddBatchFeedback(w http.ResponseWriter, r *http.Request) {
	batchID, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	in := CreateFeedbackInput{
		Scope:           FeedbackScopeBatch,
		Stage:           req.Stage,
		Action:          req.Action,
		ReasonCode:      req.ReasonCode,
		Severity:        req.Severity,
		InfluenceWeight: req.InfluenceWeight,
		Comment:         req.Comment,
		Actor:           req.Actor,
	}
	if err := validateFeedbackInput(in, FeedbackScopeBatch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	feedback, err := h.svc.AddBatchFeedback(r.Context(), batchID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Actor:        req.Actor,
		Action:       "publish.batch.add-feedback",
		ResourceType: "publish_batch",
		ResourceID:   strconv.FormatInt(batchID, 10),
		Detail: map[string]any{
			"feedback_id":           feedback.ID,
			"scope":                 feedback.Scope,
			"stage":                 feedback.Stage,
			"feedback_action":       feedback.Action,
			"reason_code":           feedback.ReasonCode,
			"influence_weight":      feedback.InfluenceWeight,
			"publish_batch_id":      feedback.PublishBatchID,
			"publish_batch_item_id": feedback.PublishBatchItemID,
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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

	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	in := CreateFeedbackInput{
		Scope:           FeedbackScopeItem,
		Stage:           req.Stage,
		Action:          req.Action,
		ReasonCode:      req.ReasonCode,
		Severity:        req.Severity,
		InfluenceWeight: req.InfluenceWeight,
		Comment:         req.Comment,
		Actor:           req.Actor,
	}
	if err := validateFeedbackInput(in, FeedbackScopeItem); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	feedback, err := h.svc.AddItemFeedback(r.Context(), batchID, itemID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Actor:        req.Actor,
		Action:       "publish.batch.add-item-feedback",
		ResourceType: "publish_batch_item",
		ResourceID:   strconv.FormatInt(itemID, 10),
		Detail: map[string]any{
			"feedback_id":           feedback.ID,
			"publish_batch_id":      batchID,
			"publish_batch_item_id": feedback.PublishBatchItemID,
			"scope":                 feedback.Scope,
			"stage":                 feedback.Stage,
			"feedback_action":       feedback.Action,
			"reason_code":           feedback.ReasonCode,
			"influence_weight":      feedback.InfluenceWeight,
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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

func (h *Handler) handleReviewDecision(w http.ResponseWriter, r *http.Request, action string, fn func(context.Context, int64, ApprovalInput) error) {
	batchID, in, err := decodeApprovalInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := fn(r.Context(), batchID, in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Actor:        in.Actor,
		Action:       action,
		ResourceType: "publish_batch",
		ResourceID:   strconv.FormatInt(batchID, 10),
		Detail: map[string]any{
			"feedback_count": len(in.Feedback),
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) handleOwnerDecision(w http.ResponseWriter, r *http.Request, action string, fn func(context.Context, int64, ApprovalInput) error) {
	batchID, in, err := decodeApprovalInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := fn(r.Context(), batchID, in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Actor:        in.Actor,
		Action:       action,
		ResourceType: "publish_batch",
		ResourceID:   strconv.FormatInt(batchID, 10),
		Detail: map[string]any{
			"feedback_count": len(in.Feedback),
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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

func (h *Handler) recordAudit(r *http.Request, event audit.Event) error {
	if h == nil || h.audit == nil {
		return nil
	}
	if event.Actor != "" {
		event.Actor = audit.NormalizeActor(event.Actor)
	}
	return h.audit.Record(r.Context(), event)
}

func validateFeedbackInput(in CreateFeedbackInput, expectedScope string) error {
	if in.Scope != expectedScope {
		return fmt.Errorf("expected scope %q, got %q", expectedScope, in.Scope)
	}
	if strings.TrimSpace(in.Stage) == "" {
		return fmt.Errorf("stage is required")
	}
	if strings.TrimSpace(in.Action) == "" {
		return fmt.Errorf("action is required")
	}
	if (in.Action == FeedbackActionReject || in.Action == FeedbackActionRework) && strings.TrimSpace(in.ReasonCode) == "" {
		return fmt.Errorf("reason_code is required for %s", in.Action)
	}
	if strings.TrimSpace(in.Severity) == "" {
		return fmt.Errorf("severity is required")
	}
	if in.InfluenceWeight <= 0 {
		return fmt.Errorf("influence_weight must be > 0")
	}
	return nil
}
