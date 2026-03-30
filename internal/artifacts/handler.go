package artifacts

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type presignArtifactRequest struct {
	TTLSeconds int `json:"ttl_seconds"`
}

func (h *Handler) CreatePackage(w http.ResponseWriter, r *http.Request) {
	var in PackageRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	artifact, err := h.svc.CreatePackageJob(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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
		writeError(w, http.StatusNotFound, err)
		return
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
	format := r.URL.Query().Get("format")
	version := r.URL.Query().Get("version")
	if format == "" || version == "" {
		writeError(w, http.StatusBadRequest, errors.New("format and version are required"))
		return
	}

	artifact, err := h.svc.ResolveArtifact(format, version)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
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

	reader, _, artifact, err := h.svc.OpenArtifactArchive(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Disposition", `attachment; filename="package.`+artifact.Format+`.tar.gz"`)
	http.ServeContent(w, r, "package."+artifact.Format+".tar.gz", time.Time{}, reader)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
