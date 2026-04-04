package datahub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/jobs"
)

type Handler struct {
	svc           *Service
	jobs          importJobCreator
	sourcePresign importSourcePresigner
}

const browseProjectID int64 = 1

func NewHandler(svc *Service) *Handler {
	return NewHandlerWithJobs(svc, nil)
}

type importJobCreator interface {
	CreateJob(in jobs.CreateJobInput) (*jobs.Job, error)
}

type importSourcePresigner func(sourceURI string, ttlSeconds int) (string, error)

func NewHandlerWithJobs(svc *Service, jobsSvc importJobCreator) *Handler {
	return NewHandlerWithJobsAndSourcePresign(svc, jobsSvc, nil)
}

func NewHandlerWithJobsAndSourcePresign(svc *Service, jobsSvc importJobCreator, presigner importSourcePresigner) *Handler {
	return &Handler{svc: svc, jobs: jobsSvc, sourcePresign: presigner}
}

func (h *Handler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	var in CreateDatasetInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	d, err := h.svc.CreateDataset(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dataset_id": d.ID})
}

func (h *Handler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	datasetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in CreateSnapshotInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	snap, err := h.svc.CreateSnapshot(datasetID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

type scanDatasetRequest struct {
	ObjectKeys []string `json:"object_keys"`
}

type importSnapshotRequest struct {
	Format               string            `json:"format"`
	SourceURI            string            `json:"source_uri,omitempty"`
	IdempotencyKey       string            `json:"idempotency_key"`
	RequiredResourceType string            `json:"required_resource_type"`
	RequiredCapabilities []string          `json:"required_capabilities"`
	Labels               map[string]string `json:"labels,omitempty"`
	Names                []string          `json:"names,omitempty"`
	Images               map[string]string `json:"images,omitempty"`
}

type importSnapshotEntryRequest struct {
	ObjectKey    string  `json:"object_key"`
	CategoryName string  `json:"category_name"`
	BBoxX        float64 `json:"bbox_x"`
	BBoxY        float64 `json:"bbox_y"`
	BBoxW        float64 `json:"bbox_w"`
	BBoxH        float64 `json:"bbox_h"`
}

type completeImportSnapshotRequest struct {
	Format    string                       `json:"format"`
	SourceURI string                       `json:"source_uri,omitempty"`
	Entries   []importSnapshotEntryRequest `json:"entries"`
}

func (h *Handler) ScanDataset(w http.ResponseWriter, r *http.Request) {
	datasetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in scanDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	added, err := h.svc.ScanDataset(datasetID, in.ObjectKeys)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added_items": added})
}

func (h *Handler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	datasetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	snaps, err := h.svc.ListSnapshots(datasetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": snaps})
}

func (h *Handler) ListDatasets(w http.ResponseWriter, _ *http.Request) {
	items, err := h.svc.ListDatasets(browseProjectID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetDatasetDetail(w http.ResponseWriter, r *http.Request) {
	datasetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	detail, err := h.svc.GetDatasetDetail(datasetID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if detail.ProjectID != browseProjectID {
		writeServiceError(w, wrapNotFound("dataset", datasetID))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) GetSnapshotDetail(w http.ResponseWriter, r *http.Request) {
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	detail, err := h.svc.GetSnapshotDetail(snapshotID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if detail.ProjectID != browseProjectID {
		writeServiceError(w, wrapNotFound("snapshot", snapshotID))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	datasetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	items, err := h.svc.ListItems(datasetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) PresignObject(w http.ResponseWriter, r *http.Request) {
	var in PresignInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	signedURL, err := h.svc.PresignObject(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": signedURL})
}

func (h *Handler) ImportSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("snapshot import queue is not configured"))
		return
	}

	var in importSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.Format == "" {
		writeError(w, http.StatusBadRequest, errors.New("format is required"))
		return
	}
	if !isSupportedImportFormat(in.Format) {
		writeError(w, http.StatusBadRequest, errors.New("unsupported format"))
		return
	}
	if in.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, errors.New("idempotency_key is required"))
		return
	}
	if in.RequiredResourceType == "" {
		in.RequiredResourceType = "cpu"
	}

	snapshot, err := h.svc.GetSnapshot(snapshotID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	payload := map[string]any{
		"format":     in.Format,
		"source_uri": in.SourceURI,
		"labels":     in.Labels,
		"names":      in.Names,
		"images":     in.Images,
	}
	if in.SourceURI != "" && h.sourcePresign != nil {
		downloadURL, err := h.sourcePresign(in.SourceURI, 300)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		payload["source_download_url"] = downloadURL
	}

	job, err := h.jobs.CreateJob(jobs.CreateJobInput{
		ProjectID:            1,
		DatasetID:            snapshot.DatasetID,
		SnapshotID:           snapshot.ID,
		JobType:              "snapshot-import",
		RequiredResourceType: in.RequiredResourceType,
		RequiredCapabilities: in.RequiredCapabilities,
		IdempotencyKey:       in.IdempotencyKey,
		Payload:              payload,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":      job.ID,
		"status":      job.Status,
		"dataset_id":  snapshot.DatasetID,
		"snapshot_id": snapshot.ID,
	})
}

func (h *Handler) CompleteImportSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in completeImportSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !isSupportedImportFormat(in.Format) {
		writeError(w, http.StatusBadRequest, errors.New("unsupported format"))
		return
	}

	entries := make([]ImportedAnnotation, 0, len(in.Entries))
	for _, entry := range in.Entries {
		entries = append(entries, ImportedAnnotation{
			ObjectKey:    entry.ObjectKey,
			CategoryName: entry.CategoryName,
			BBoxX:        entry.BBoxX,
			BBoxY:        entry.BBoxY,
			BBoxW:        entry.BBoxW,
			BBoxH:        entry.BBoxH,
		})
	}

	report, err := h.svc.ImportSnapshot(snapshotID, ImportSnapshotInput{
		Format:    in.Format,
		SourceURI: in.SourceURI,
		Entries:   entries,
	})
	if err != nil {
		writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func writeImportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}
