package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type zeroShotRequest struct {
	ProjectID            int64          `json:"project_id"`
	DatasetID            int64          `json:"dataset_id"`
	SnapshotID           int64          `json:"snapshot_id"`
	Prompt               string         `json:"prompt"`
	Provider             map[string]any `json:"provider"`
	IdempotencyKey       string         `json:"idempotency_key"`
	RequiredResourceType string         `json:"required_resource_type"`
	RequiredCapabilities []string       `json:"required_capabilities"`
}

type videoExtractRequest struct {
	ProjectID            int64          `json:"project_id"`
	DatasetID            int64          `json:"dataset_id"`
	FPS                  int            `json:"fps"`
	Provider             map[string]any `json:"provider"`
	IdempotencyKey       string         `json:"idempotency_key"`
	RequiredResourceType string         `json:"required_resource_type"`
	RequiredCapabilities []string       `json:"required_capabilities"`
}

type cleaningRequest struct {
	ProjectID            int64          `json:"project_id"`
	DatasetID            int64          `json:"dataset_id"`
	SnapshotID           int64          `json:"snapshot_id"`
	Rules                map[string]any `json:"rules"`
	IdempotencyKey       string         `json:"idempotency_key"`
	RequiredResourceType string         `json:"required_resource_type"`
	RequiredCapabilities []string       `json:"required_capabilities"`
}

type workerHeartbeatRequest struct {
	WorkerID     string `json:"worker_id"`
	LeaseSeconds int    `json:"lease_seconds"`
}

type workerProgressRequest struct {
	WorkerID       string `json:"worker_id"`
	TotalItems     int    `json:"total_items"`
	SucceededItems int    `json:"succeeded_items"`
	FailedItems    int    `json:"failed_items"`
}

type workerEventRequest struct {
	ItemID     *int64         `json:"item_id,omitempty"`
	EventLevel string         `json:"event_level"`
	EventType  string         `json:"event_type"`
	Message    string         `json:"message"`
	Detail     map[string]any `json:"detail_json"`
}

type workerTerminalRequest struct {
	WorkerID       string         `json:"worker_id"`
	Status         string         `json:"status"`
	TotalItems     int            `json:"total_items"`
	SucceededItems int            `json:"succeeded_items"`
	FailedItems    int            `json:"failed_items"`
	ResultRef      map[string]any `json:"result_ref,omitempty"`
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
	provider, err := normalizeProvider(in.Provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload := map[string]any{
		"dataset_id":  in.DatasetID,
		"snapshot_id": in.SnapshotID,
		"prompt":      in.Prompt,
	}
	if provider != nil {
		payload["provider"] = provider
	}
	job, err := h.svc.CreateJob(CreateJobInput{
		ProjectID:            in.ProjectID,
		DatasetID:            in.DatasetID,
		SnapshotID:           in.SnapshotID,
		JobType:              "zero-shot",
		RequiredResourceType: in.RequiredResourceType,
		RequiredCapabilities: in.RequiredCapabilities,
		IdempotencyKey:       in.IdempotencyKey,
		Payload:              payload,
	})
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
	provider, err := normalizeProvider(in.Provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload := map[string]any{
		"dataset_id": in.DatasetID,
		"fps":        in.FPS,
	}
	if provider != nil {
		payload["provider"] = provider
	}
	job, err := h.svc.CreateJob(CreateJobInput{
		ProjectID:            in.ProjectID,
		DatasetID:            in.DatasetID,
		JobType:              "video-extract",
		RequiredResourceType: in.RequiredResourceType,
		RequiredCapabilities: in.RequiredCapabilities,
		IdempotencyKey:       in.IdempotencyKey,
		Payload:              payload,
	})
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
	job, err := h.svc.CreateJob(CreateJobInput{
		ProjectID:            in.ProjectID,
		DatasetID:            in.DatasetID,
		SnapshotID:           in.SnapshotID,
		JobType:              "cleaning",
		RequiredResourceType: in.RequiredResourceType,
		RequiredCapabilities: in.RequiredCapabilities,
		IdempotencyKey:       in.IdempotencyKey,
		Payload:              payload,
	})
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
		"dataset_id":             job.DatasetID,
		"snapshot_id":            job.SnapshotID,
		"job_type":               job.JobType,
		"status":                 job.Status,
		"payload":                job.Payload,
		"resource_lane":          laneFor(job.RequiredResourceType),
		"required_resource_type": job.RequiredResourceType,
		"required_capabilities":  job.RequiredCapabilities,
		"idempotency_key":        job.IdempotencyKey,
		"worker_id":              job.WorkerID,
		"lease_until":            job.LeaseUntil,
		"retry_count":            job.RetryCount,
		"error_code":             job.ErrorCode,
		"error_msg":              job.ErrorMsg,
		"result_type":            job.ResultType,
		"result_count":           job.ResultCount,
		"result_ref":             job.ResultRef,
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

func (h *Handler) ReportHeartbeat(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in workerHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.ReportHeartbeat(jobID, in.WorkerID, in.LeaseSeconds); err != nil {
		writeCallbackError(w, err)
		return
	}
	writeJobStatus(w, h.svc, jobID)
}

func (h *Handler) ReportProgress(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in workerProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.ReportProgress(jobID, in.WorkerID, in.TotalItems, in.SucceededItems, in.FailedItems); err != nil {
		writeCallbackError(w, err)
		return
	}
	writeJobStatus(w, h.svc, jobID)
}

func (h *Handler) ReportItemError(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in workerEventRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.EventType == "" {
		in.EventType = "item_failed"
	}
	if in.EventLevel == "" {
		if in.EventType == "item_failed" {
			in.EventLevel = "error"
		} else {
			in.EventLevel = "warn"
		}
	}
	if err := h.svc.ReportEvent(jobID, in.ItemID, in.EventLevel, in.EventType, in.Message, in.Detail); err != nil {
		writeCallbackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "status": "accepted"})
}

func (h *Handler) ReportTerminal(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(chi.URLParam(r, "job_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in workerTerminalRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.ReportTerminal(jobID, in.WorkerID, in.Status, in.TotalItems, in.SucceededItems, in.FailedItems, in.ResultRef); err != nil {
		writeCallbackError(w, err)
		return
	}
	writeJobStatus(w, h.svc, jobID)
}

func writeJobStatus(w http.ResponseWriter, svc *Service, jobID int64) {
	job, ok := svc.GetJob(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("job %d not found", jobID))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":          job.ID,
		"status":          job.Status,
		"worker_id":       job.WorkerID,
		"total_items":     job.TotalItems,
		"succeeded_items": job.SucceededItems,
		"failed_items":    job.FailedItems,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func writeCallbackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func normalizeProvider(raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	providerType := strings.TrimSpace(strings.ToLower(fmt.Sprint(raw["type"])))
	if providerType == "" {
		return nil, errors.New("provider.type is required")
	}
	if providerType != "command" {
		return nil, fmt.Errorf("unsupported provider.type %q", providerType)
	}

	argv, err := normalizeProviderArgv(raw["argv"])
	if err != nil {
		return nil, err
	}
	provider := map[string]any{
		"type": "command",
		"argv": argv,
	}
	timeoutSeconds, ok, err := normalizeProviderTimeout(raw["timeout_seconds"])
	if err != nil {
		return nil, err
	}
	if ok {
		provider["timeout_seconds"] = timeoutSeconds
	}
	return provider, nil
}

func normalizeProviderArgv(raw any) ([]string, error) {
	switch argv := raw.(type) {
	case []string:
		if len(argv) == 0 {
			return nil, errors.New("provider.argv must contain at least one command argument")
		}
		return argv, nil
	case []any:
		if len(argv) == 0 {
			return nil, errors.New("provider.argv must contain at least one command argument")
		}
		out := make([]string, 0, len(argv))
		for _, value := range argv {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text == "" {
				return nil, errors.New("provider.argv entries must be non-empty strings")
			}
			out = append(out, text)
		}
		return out, nil
	default:
		return nil, errors.New("provider.argv must be an array of command arguments")
	}
}

func normalizeProviderTimeout(raw any) (float64, bool, error) {
	if raw == nil {
		return 0, false, nil
	}

	var timeout float64
	switch value := raw.(type) {
	case float64:
		timeout = value
	case float32:
		timeout = float64(value)
	case int:
		timeout = float64(value)
	case int8:
		timeout = float64(value)
	case int16:
		timeout = float64(value)
	case int32:
		timeout = float64(value)
	case int64:
		timeout = float64(value)
	case uint:
		timeout = float64(value)
	case uint8:
		timeout = float64(value)
	case uint16:
		timeout = float64(value)
	case uint32:
		timeout = float64(value)
	case uint64:
		timeout = float64(value)
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, false, errors.New("provider.timeout_seconds must be a positive number")
		}
		timeout = parsed
	default:
		return 0, false, errors.New("provider.timeout_seconds must be a positive number")
	}

	if math.IsNaN(timeout) || math.IsInf(timeout, 0) || timeout <= 0 {
		return 0, false, errors.New("provider.timeout_seconds must be a positive number")
	}
	return timeout, true, nil
}

func requireIdempotencyKey(key string) error {
	if key == "" {
		return errors.New("idempotency_key is required")
	}
	return nil
}
