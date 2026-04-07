package datahub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/audit"
	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/observability"
)

type Handler struct {
	svc           *Service
	jobs          importJobCreator
	sourcePresign importSourcePresigner
	audit         audit.Logger
}

const (
	browseProjectID                       int64 = 1
	importSnapshotRequestMaxBytes               = 8 << 20
	completeImportSnapshotRequestMaxBytes       = 8 << 20
)

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
	return NewHandlerWithJobsAndSourcePresignAndAudit(svc, jobsSvc, presigner, nil)
}

func NewHandlerWithJobsAndSourcePresignAndAudit(svc *Service, jobsSvc importJobCreator, presigner importSourcePresigner, auditLogger audit.Logger) *Handler {
	return &Handler{svc: svc, jobs: jobsSvc, sourcePresign: presigner, audit: auditLogger}
}

func (h *Handler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	var in CreateDatasetInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := auth.RequireProjectAccess(r.Context(), in.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	d, err := h.svc.CreateDataset(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Action:       "dataset.create",
		ResourceType: "dataset",
		ResourceID:   strconv.FormatInt(d.ID, 10),
		Detail: map[string]any{
			"project_id": d.ProjectID,
			"name":       d.Name,
			"bucket":     d.Bucket,
			"prefix":     d.Prefix,
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	if err := h.authorizeDatasetProject(r, datasetID); err != nil {
		writeProjectAccessError(w, err)
		return
	}

	snap, err := h.svc.CreateSnapshot(datasetID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Action:       "snapshot.create",
		ResourceType: "snapshot",
		ResourceID:   strconv.FormatInt(snap.ID, 10),
		Detail: map[string]any{
			"dataset_id": snap.DatasetID,
			"version":    snap.Version,
			"note":       snap.Note,
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	if err := h.authorizeDatasetProject(r, datasetID); err != nil {
		writeProjectAccessError(w, err)
		return
	}

	added, err := h.svc.ScanDataset(datasetID, in.ObjectKeys)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordAudit(r, audit.Event{
		Action:       "dataset.scan",
		ResourceType: "dataset",
		ResourceID:   strconv.FormatInt(datasetID, 10),
		Detail: map[string]any{
			"added_items":       added,
			"object_keys_count": len(in.ObjectKeys),
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	if err := h.authorizeDatasetProject(r, datasetID); err != nil {
		writeProjectReadError(w, err, "dataset", datasetID)
		return
	}

	snaps, err := h.svc.ListSnapshots(datasetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": snaps})
}

func (h *Handler) ListDatasets(w http.ResponseWriter, r *http.Request) {
	projectIDs := browseProjectScopes(r)
	items := make([]DatasetSummary, 0)
	for _, projectID := range projectIDs {
		projectItems, err := h.svc.ListDatasets(projectID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		items = append(items, projectItems...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ProjectID == items[j].ProjectID {
			return items[i].ID < items[j].ID
		}
		return items[i].ProjectID < items[j].ProjectID
	})
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
	if err := auth.RequireProjectAccess(r.Context(), detail.ProjectID); err != nil {
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
	if err := auth.RequireProjectAccess(r.Context(), detail.ProjectID); err != nil {
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
	if err := h.authorizeDatasetProject(r, datasetID); err != nil {
		writeProjectReadError(w, err, "dataset", datasetID)
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
	if err := h.authorizeDatasetProject(r, in.DatasetID); err != nil {
		writeProjectAccessError(w, err)
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
	if err := h.authorizeSnapshotProject(r, snapshotID); err != nil {
		writeProjectAccessError(w, err)
		return
	}

	var in importSnapshotRequest
	if err := decodeJSONBodyWithLimit(w, r, &in, importSnapshotRequestMaxBytes); err != nil {
		writeBodyDecodeError(w, err, importSnapshotRequestMaxBytes)
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
	datasetDetail, err := h.svc.GetDatasetDetail(snapshot.DatasetID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	payload := map[string]any{
		"format":     in.Format,
		"source_uri": in.SourceURI,
		"labels":     in.Labels,
		"names":      in.Names,
		"images":     in.Images,
	}
	if traceID := observability.TraceIDFromRequest(r); traceID != "" {
		payload["trace_id"] = traceID
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
		ProjectID:            datasetDetail.ProjectID,
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
	if err := h.recordAudit(r, audit.Event{
		Action:       "job.create.snapshot-import",
		ResourceType: "job",
		ResourceID:   strconv.FormatInt(job.ID, 10),
		Detail: map[string]any{
			"dataset_id":  snapshot.DatasetID,
			"snapshot_id": snapshot.ID,
			"format":      in.Format,
		},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	if err := decodeJSONBodyWithLimit(w, r, &in, completeImportSnapshotRequestMaxBytes); err != nil {
		writeBodyDecodeError(w, err, completeImportSnapshotRequestMaxBytes)
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

func decodeJSONBodyWithLimit(w http.ResponseWriter, r *http.Request, out any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	return json.NewDecoder(r.Body).Decode(out)
}

func writeBodyDecodeError(w http.ResponseWriter, err error, maxBytes int64) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", maxBytes))
		return
	}
	writeError(w, http.StatusBadRequest, err)
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
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func writeProjectAccessError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrForbidden) {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeServiceError(w, err)
}

func writeProjectReadError(w http.ResponseWriter, err error, resource string, id int64) {
	if errors.Is(err, auth.ErrForbidden) {
		writeServiceError(w, wrapNotFound(resource, id))
		return
	}
	writeServiceError(w, err)
}

func (h *Handler) authorizeDatasetProject(r *http.Request, datasetID int64) error {
	detail, err := h.svc.GetDatasetDetail(datasetID)
	if err != nil {
		return err
	}
	return auth.RequireProjectAccess(r.Context(), detail.ProjectID)
}

func (h *Handler) authorizeSnapshotProject(r *http.Request, snapshotID int64) error {
	snapshot, err := h.svc.GetSnapshot(snapshotID)
	if err != nil {
		return err
	}
	return h.authorizeDatasetProject(r, snapshot.DatasetID)
}

func (h *Handler) recordAudit(r *http.Request, event audit.Event) error {
	if h == nil || h.audit == nil {
		return nil
	}
	return h.audit.Record(r.Context(), event)
}

func browseProjectScopes(r *http.Request) []int64 {
	if identity, ok := auth.IdentityFromContext(r.Context()); ok && len(identity.ProjectIDs) > 0 {
		return append([]int64(nil), identity.ProjectIDs...)
	}
	return []int64{browseProjectID}
}
