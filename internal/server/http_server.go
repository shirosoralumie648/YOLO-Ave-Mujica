package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/jobs"
)

type HTTPServer struct {
	Handler http.Handler
}

type ReadyCheck func(ctx context.Context) error

type DataHubRoutes struct {
	CreateDataset          http.HandlerFunc
	ScanDataset            http.HandlerFunc
	CreateSnapshot         http.HandlerFunc
	ListSnapshots          http.HandlerFunc
	ListItems              http.HandlerFunc
	PresignObject          http.HandlerFunc
	ImportSnapshot         http.HandlerFunc
	CompleteImportSnapshot http.HandlerFunc
}

type JobRoutes struct {
	CreateZeroShot     http.HandlerFunc
	CreateVideoExtract http.HandlerFunc
	CreateCleaning     http.HandlerFunc
	GetJob             http.HandlerFunc
	ListEvents         http.HandlerFunc
	ReportHeartbeat    http.HandlerFunc
	ReportProgress     http.HandlerFunc
	ReportItemError    http.HandlerFunc
	ReportTerminal     http.HandlerFunc
}

type VersioningRoutes struct {
	DiffSnapshots http.HandlerFunc
}

type ReviewRoutes struct {
	ListCandidates  http.HandlerFunc
	AcceptCandidate http.HandlerFunc
	RejectCandidate http.HandlerFunc
}

type ArtifactRoutes struct {
	CreatePackage    http.HandlerFunc
	GetArtifact      http.HandlerFunc
	PresignArtifact  http.HandlerFunc
	ResolveArtifact  http.HandlerFunc
	ExportSnapshot   http.HandlerFunc
	CompleteArtifact http.HandlerFunc
	DownloadArtifact http.HandlerFunc
}

type Modules struct {
	DataHub     DataHubRoutes
	Jobs        JobRoutes
	Versioning  VersioningRoutes
	Review      ReviewRoutes
	Artifacts   ArtifactRoutes
	ReadyChecks []ReadyCheck
}

// NewHTTPServer builds a control-plane HTTP server with base health routes.
func NewHTTPServer() *HTTPServer {
	return NewHTTPServerWithModules(Modules{})
}

// NewHTTPServerWithDataHub optionally wires Data Hub APIs when a handler is provided.
func NewHTTPServerWithDataHub(dataHubHandler *datahub.Handler) *HTTPServer {
	var dataHubRoutes DataHubRoutes
	if dataHubHandler != nil {
		dataHubRoutes = DataHubRoutes{
			CreateDataset:          dataHubHandler.CreateDataset,
			ScanDataset:            dataHubHandler.ScanDataset,
			CreateSnapshot:         dataHubHandler.CreateSnapshot,
			ListSnapshots:          dataHubHandler.ListSnapshots,
			ListItems:              dataHubHandler.ListItems,
			PresignObject:          dataHubHandler.PresignObject,
			ImportSnapshot:         dataHubHandler.ImportSnapshot,
			CompleteImportSnapshot: dataHubHandler.CompleteImportSnapshot,
		}
	}
	return NewHTTPServerWithModules(Modules{DataHub: dataHubRoutes})
}

// NewHTTPServerWithDataHubAndJobs wires Data Hub and Job APIs for local runtime.
func NewHTTPServerWithDataHubAndJobs(dataHubHandler *datahub.Handler, jobsHandler *jobs.Handler) *HTTPServer {
	var dataHubRoutes DataHubRoutes

	if dataHubHandler != nil {
		dataHubRoutes = DataHubRoutes{
			CreateDataset:          dataHubHandler.CreateDataset,
			ScanDataset:            dataHubHandler.ScanDataset,
			CreateSnapshot:         dataHubHandler.CreateSnapshot,
			ListSnapshots:          dataHubHandler.ListSnapshots,
			ListItems:              dataHubHandler.ListItems,
			PresignObject:          dataHubHandler.PresignObject,
			ImportSnapshot:         dataHubHandler.ImportSnapshot,
			CompleteImportSnapshot: dataHubHandler.CompleteImportSnapshot,
		}
	}
	var jobRoutes JobRoutes
	if jobsHandler != nil {
		jobRoutes = JobRoutes{
			CreateZeroShot:     jobsHandler.CreateZeroShot,
			CreateVideoExtract: jobsHandler.CreateVideoExtract,
			CreateCleaning:     jobsHandler.CreateCleaning,
			GetJob:             jobsHandler.GetJob,
			ListEvents:         jobsHandler.ListEvents,
			ReportHeartbeat:    jobsHandler.ReportHeartbeat,
			ReportProgress:     jobsHandler.ReportProgress,
			ReportItemError:    jobsHandler.ReportItemError,
			ReportTerminal:     jobsHandler.ReportTerminal,
		}
	}
	return NewHTTPServerWithModules(Modules{
		DataHub: dataHubRoutes,
		Jobs:    jobRoutes,
	})
}

// NewHTTPServerWithModules wires all MVP route groups.
// Handlers left unset return 501 to keep route shape stable during incremental delivery.
func NewHTTPServerWithModules(m Modules) *HTTPServer {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		for _, check := range m.ReadyChecks {
			if check == nil {
				continue
			}
			if err := check(context.Background()); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/datasets", handlerOrNotImplemented(m.DataHub.CreateDataset))
		r.Post("/datasets/{id}/scan", handlerOrNotImplemented(m.DataHub.ScanDataset))
		r.Post("/datasets/{id}/snapshots", handlerOrNotImplemented(m.DataHub.CreateSnapshot))
		r.Get("/datasets/{id}/snapshots", handlerOrNotImplemented(m.DataHub.ListSnapshots))
		r.Get("/datasets/{id}/items", handlerOrNotImplemented(m.DataHub.ListItems))
		r.Post("/objects/presign", handlerOrNotImplemented(m.DataHub.PresignObject))
		r.Post("/snapshots/{id}/import", handlerOrNotImplemented(m.DataHub.ImportSnapshot))

		r.Post("/jobs/zero-shot", handlerOrNotImplemented(m.Jobs.CreateZeroShot))
		r.Post("/jobs/video-extract", handlerOrNotImplemented(m.Jobs.CreateVideoExtract))
		r.Post("/jobs/cleaning", handlerOrNotImplemented(m.Jobs.CreateCleaning))
		r.Get("/jobs/{job_id}", handlerOrNotImplemented(m.Jobs.GetJob))
		r.Get("/jobs/{job_id}/events", handlerOrNotImplemented(m.Jobs.ListEvents))

		r.Post("/snapshots/diff", handlerOrNotImplemented(m.Versioning.DiffSnapshots))

		r.Get("/review/candidates", handlerOrNotImplemented(m.Review.ListCandidates))
		r.Post("/review/candidates/{id}/accept", handlerOrNotImplemented(m.Review.AcceptCandidate))
		r.Post("/review/candidates/{id}/reject", handlerOrNotImplemented(m.Review.RejectCandidate))

		r.Post("/artifacts/packages", handlerOrNotImplemented(m.Artifacts.CreatePackage))
		r.Post("/snapshots/{id}/export", handlerOrNotImplemented(m.Artifacts.ExportSnapshot))
		r.Get("/artifacts/resolve", handlerOrNotImplemented(m.Artifacts.ResolveArtifact))
		r.Get("/artifacts/{id}", handlerOrNotImplemented(m.Artifacts.GetArtifact))
		r.Get("/artifacts/{id}/download", handlerOrNotImplemented(m.Artifacts.DownloadArtifact))
		r.Post("/artifacts/{id}/presign", handlerOrNotImplemented(m.Artifacts.PresignArtifact))
	})

	r.Route("/internal", func(r chi.Router) {
		r.Post("/jobs/{job_id}/heartbeat", handlerOrNotImplemented(m.Jobs.ReportHeartbeat))
		r.Post("/jobs/{job_id}/progress", handlerOrNotImplemented(m.Jobs.ReportProgress))
		r.Post("/jobs/{job_id}/events", handlerOrNotImplemented(m.Jobs.ReportItemError))
		r.Post("/jobs/{job_id}/complete", handlerOrNotImplemented(m.Jobs.ReportTerminal))
		r.Post("/snapshots/{id}/import", handlerOrNotImplemented(m.DataHub.CompleteImportSnapshot))
		r.Post("/artifacts/{id}/complete", handlerOrNotImplemented(m.Artifacts.CompleteArtifact))
	})

	return &HTTPServer{Handler: r}
}

func handlerOrNotImplemented(h http.HandlerFunc) http.HandlerFunc {
	if h != nil {
		return h
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}
