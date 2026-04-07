package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/audit"
	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/observability"
)

type Handler struct {
	svc   *Service
	jobs  packageJobCreator
	audit audit.Logger
}

func NewHandler(svc *Service) *Handler {
	return NewHandlerWithJobs(svc, nil)
}

type packageJobCreator interface {
	CreateJob(in jobs.CreateJobInput) (*jobs.Job, error)
}

func NewHandlerWithJobs(svc *Service, jobsSvc packageJobCreator) *Handler {
	return NewHandlerWithJobsAndAudit(svc, jobsSvc, nil)
}

func NewHandlerWithJobsAndAudit(svc *Service, jobsSvc packageJobCreator, auditLogger audit.Logger) *Handler {
	return &Handler{svc: svc, jobs: jobsSvc, audit: auditLogger}
}

type presignArtifactRequest struct {
	TTLSeconds int `json:"ttl_seconds"`
}

type completeArtifactRequest struct {
	Entries []BundleEntry `json:"entries"`
}

func (h *Handler) CreatePackage(w http.ResponseWriter, r *http.Request) {
	var in PackageRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	projectID, err := h.svc.resolveProjectID(r.Context(), in.DatasetID, in.SnapshotID, in.ProjectID)
	if err != nil {
		writeArtifactProjectResolutionError(w, err)
		return
	}
	in.ProjectID = projectID
	if err := auth.RequireProjectAccess(r.Context(), in.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	if h.jobs != nil {
		artifact, job, err := h.queuePackageJob(in, observability.TraceIDFromRequest(r))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := h.recordPackageRequestAudit(r, artifact, job); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"job_id":      job.ID,
			"artifact_id": artifact.ID,
			"status":      job.Status,
		})
		return
	}

	artifact, err := h.svc.CreatePackageJob(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordPackageRequestAudit(r, artifact, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":      artifact.ID,
		"artifact_id": artifact.ID,
		"status":      artifact.Status,
	})
}

func (h *Handler) GetArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a, err := h.svc.GetArtifact(id)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	if err := auth.RequireProjectAccess(r.Context(), a.ProjectID); err != nil {
		writeArtifactError(w, wrapArtifactNotFound(id))
		return
	}
	if a.Status == StatusPending || a.Status == StatusQueued {
		a.Entries = BuildBundleEntries(a)
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) PresignArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in presignArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	artifact, err := h.svc.GetArtifact(id)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	if err := auth.RequireProjectAccess(r.Context(), artifact.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	url, err := h.svc.PresignArtifact(id, in.TTLSeconds)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func (h *Handler) ResolveArtifact(w http.ResponseWriter, r *http.Request) {
	dataset := r.URL.Query().Get("dataset")
	format := r.URL.Query().Get("format")
	version := r.URL.Query().Get("version")
	if format == "" || version == "" {
		writeError(w, http.StatusBadRequest, errors.New("format and version are required"))
		return
	}

	artifact, err := h.svc.ResolveArtifact(dataset, format, version)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := auth.RequireProjectAccess(r.Context(), artifact.ProjectID); err != nil {
		if dataset != "" {
			writeError(w, http.StatusNotFound, fmt.Errorf("artifact %s/%s@%s not found", dataset, format, version))
			return
		}
		writeError(w, http.StatusNotFound, fmt.Errorf("artifact %s@%s not found", format, version))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Artifact
		DownloadURL string `json:"download_url"`
	}{
		Artifact:    artifact,
		DownloadURL: "/v1/artifacts/" + strconv.FormatInt(artifact.ID, 10) + "/download",
	})
}

func (h *Handler) DownloadArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	artifact, err := h.svc.GetArtifact(id)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	if err := auth.RequireProjectAccess(r.Context(), artifact.ProjectID); err != nil {
		writeArtifactError(w, wrapArtifactNotFound(id))
		return
	}

	reader, _, artifact, err := h.svc.OpenArtifactArchive(r.Context(), id)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Disposition", `attachment; filename="package.`+artifact.Format+`.tar.gz"`)
	http.ServeContent(w, r, "package."+artifact.Format+".tar.gz", time.Time{}, reader)
}

func (h *Handler) ExportSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in PackageRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	in.SnapshotID = snapshotID
	projectID, err := h.svc.resolveProjectID(r.Context(), in.DatasetID, in.SnapshotID, in.ProjectID)
	if err != nil {
		writeArtifactProjectResolutionError(w, err)
		return
	}
	in.ProjectID = projectID
	if err := auth.RequireProjectAccess(r.Context(), in.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	if h.jobs != nil {
		artifact, job, err := h.queuePackageJob(in, observability.TraceIDFromRequest(r))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := h.recordPackageRequestAudit(r, artifact, job); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "artifact_id": artifact.ID, "status": job.Status})
		return
	}

	artifact, err := h.svc.CreatePackageJob(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.recordPackageRequestAudit(r, artifact, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": artifact.ID, "artifact_id": artifact.ID, "status": artifact.Status})
}

func (h *Handler) CompleteArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in completeArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	artifact, err := h.svc.CompleteArtifact(id, in.Entries)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (h *Handler) queuePackageJob(in PackageRequest, traceID string) (Artifact, *jobs.Job, error) {
	artifact, err := h.svc.CreateArtifact(in, StatusQueued)
	if err != nil {
		return Artifact{}, nil, err
	}

	payload := map[string]any{
		"artifact_id":    artifact.ID,
		"dataset_id":     artifact.DatasetID,
		"snapshot_id":    artifact.SnapshotID,
		"format":         artifact.Format,
		"version":        artifact.Version,
		"label_map_json": artifact.LabelMapJSON,
		"names":          bundleNames(artifact.LabelMapJSON),
	}
	if traceID != "" {
		payload["trace_id"] = traceID
	}

	job, err := h.jobs.CreateJob(jobs.CreateJobInput{
		ProjectID:            artifact.ProjectID,
		DatasetID:            artifact.DatasetID,
		SnapshotID:           artifact.SnapshotID,
		JobType:              "artifact-package",
		RequiredResourceType: "cpu",
		IdempotencyKey:       fmt.Sprintf("artifact-package-%d", artifact.ID),
		Payload:              payload,
	})
	if err != nil {
		return Artifact{}, nil, err
	}
	artifact, err = h.svc.LinkArtifactJob(artifact.ID, job.ID)
	if err != nil {
		return Artifact{}, nil, err
	}
	return artifact, job, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func writeArtifactError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrArtifactNotReady):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func writeArtifactProjectResolutionError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func (h *Handler) recordPackageRequestAudit(r *http.Request, artifact Artifact, job *jobs.Job) error {
	if h == nil || h.audit == nil {
		return nil
	}

	detail := map[string]any{
		"project_id":  artifact.ProjectID,
		"dataset_id":  artifact.DatasetID,
		"snapshot_id": artifact.SnapshotID,
		"format":      artifact.Format,
		"version":     artifact.Version,
		"status":      artifact.Status,
	}
	if job != nil {
		detail["job_id"] = job.ID
		detail["job_status"] = job.Status
	}

	return h.audit.Record(r.Context(), audit.Event{
		Action:       "artifact.package.request",
		ResourceType: "artifact",
		ResourceID:   strconv.FormatInt(artifact.ID, 10),
		Detail:       detail,
	})
}
