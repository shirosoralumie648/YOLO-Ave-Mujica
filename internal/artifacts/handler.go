package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/jobs"
)

type Handler struct {
	svc  *Service
	jobs packageJobCreator
}

func NewHandler(svc *Service) *Handler {
	return NewHandlerWithJobs(svc, nil)
}

type packageJobCreator interface {
	CreateJob(in jobs.CreateJobInput) (*jobs.Job, error)
}

func NewHandlerWithJobs(svc *Service, jobsSvc packageJobCreator) *Handler {
	return &Handler{svc: svc, jobs: jobsSvc}
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

	if h.jobs != nil {
		h.queuePackageJob(w, in)
		return
	}

	jobID, artifactID, err := h.svc.CreatePackageJob(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "artifact_id": artifactID, "status": "queued"})
}

func (h *Handler) GetArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a, err := h.svc.GetArtifact(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if a.Status == "pending" {
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
	url, err := h.svc.PresignArtifact(id, in.TTLSeconds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	writeJSON(w, http.StatusOK, artifact)
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

	if h.jobs != nil {
		h.queuePackageJob(w, in)
		return
	}

	jobID, artifactID, err := h.svc.CreatePackageJob(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "artifact_id": artifactID, "status": "queued"})
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
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (h *Handler) queuePackageJob(w http.ResponseWriter, in PackageRequest) {
	artifact, err := h.svc.CreateArtifact(in, "queued")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	job, err := h.jobs.CreateJob(jobs.CreateJobInput{
		ProjectID:            artifact.ProjectID,
		DatasetID:            artifact.DatasetID,
		SnapshotID:           artifact.SnapshotID,
		JobType:              "artifact-package",
		RequiredResourceType: "cpu",
		IdempotencyKey:       fmt.Sprintf("artifact-package-%d", artifact.ID),
		Payload: map[string]any{
			"artifact_id":    artifact.ID,
			"dataset_id":     artifact.DatasetID,
			"snapshot_id":    artifact.SnapshotID,
			"format":         artifact.Format,
			"version":        artifact.Version,
			"label_map_json": artifact.LabelMapJSON,
			"names":          bundleNames(artifact.LabelMapJSON),
		},
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "artifact_id": artifact.ID, "status": job.Status})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
