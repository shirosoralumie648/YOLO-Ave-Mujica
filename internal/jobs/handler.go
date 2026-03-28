package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
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

type zeroShotRequest struct {
	ProjectID            int64  `json:"project_id"`
	DatasetID            int64  `json:"dataset_id"`
	SnapshotID           int64  `json:"snapshot_id"`
	Prompt               string `json:"prompt"`
	IdempotencyKey       string `json:"idempotency_key"`
	RequiredResourceType string `json:"required_resource_type"`
}

type videoExtractRequest struct {
	ProjectID            int64  `json:"project_id"`
	DatasetID            int64  `json:"dataset_id"`
	FPS                  int    `json:"fps"`
	IdempotencyKey       string `json:"idempotency_key"`
	RequiredResourceType string `json:"required_resource_type"`
}

type cleaningRequest struct {
	ProjectID            int64          `json:"project_id"`
	DatasetID            int64          `json:"dataset_id"`
	SnapshotID           int64          `json:"snapshot_id"`
	Rules                map[string]any `json:"rules"`
	IdempotencyKey       string         `json:"idempotency_key"`
	RequiredResourceType string         `json:"required_resource_type"`
}

func (h *Handler) CreateZeroShot(w http.ResponseWriter, r *http.Request) {
	var in zeroShotRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload := map[string]any{
		"dataset_id":  in.DatasetID,
		"snapshot_id": in.SnapshotID,
		"prompt":      in.Prompt,
	}
	job, err := h.svc.CreateJob(in.ProjectID, "zero-shot", in.RequiredResourceType, in.IdempotencyKey, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "status": job.Status})
}

func (h *Handler) CreateVideoExtract(w http.ResponseWriter, r *http.Request) {
	var in videoExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload := map[string]any{
		"dataset_id": in.DatasetID,
		"fps":        in.FPS,
	}
	job, err := h.svc.CreateJob(in.ProjectID, "video-extract", in.RequiredResourceType, in.IdempotencyKey, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "status": job.Status})
}

func (h *Handler) CreateCleaning(w http.ResponseWriter, r *http.Request) {
	var in cleaningRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload := map[string]any{
		"dataset_id":  in.DatasetID,
		"snapshot_id": in.SnapshotID,
		"rules":       in.Rules,
	}
	job, err := h.svc.CreateJob(in.ProjectID, "cleaning", in.RequiredResourceType, in.IdempotencyKey, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "status": job.Status})
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, ok := h.svc.GetJob(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("job %d not found", id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                     job.ID,
		"project_id":             job.ProjectID,
		"job_type":               job.JobType,
		"status":                 job.Status,
		"required_resource_type": job.RequiredResourceType,
		"idempotency_key":        job.IdempotencyKey,
		"total_items":            job.TotalItems,
		"succeeded_items":        job.SucceededItems,
		"failed_items":           job.FailedItems,
	})
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": h.svc.ListEvents(id)})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func requireIdempotencyKey(key string) error {
	if key == "" {
		return errors.New("idempotency_key is required")
	}
	return nil
}
