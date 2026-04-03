package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/jobs"
)

// HTTPServer owns the root handler for the control-plane HTTP surface.
type HTTPServer struct {
	Handler http.Handler
}

// ReadyCheck reports whether a required runtime dependency is ready for traffic.
type ReadyCheck func(ctx context.Context) error

// DataHubRoutes groups handlers for dataset and object-management endpoints.
type DataHubRoutes struct {
	CreateDataset          http.HandlerFunc
	ListDatasets           http.HandlerFunc
	GetDatasetDetail       http.HandlerFunc
	GetSnapshotDetail      http.HandlerFunc
	ScanDataset            http.HandlerFunc
	CreateSnapshot         http.HandlerFunc
	ListSnapshots          http.HandlerFunc
	ListItems              http.HandlerFunc
	PresignObject          http.HandlerFunc
	ImportSnapshot         http.HandlerFunc
	CompleteImportSnapshot http.HandlerFunc
}

// JobRoutes groups handlers for asynchronous job creation and inspection.
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

// VersioningRoutes groups handlers for snapshot diff operations.
type VersioningRoutes struct {
	DiffSnapshots http.HandlerFunc
}

// ReviewRoutes groups handlers for review candidate listing and decisions.
type ReviewRoutes struct {
	ListCandidates  http.HandlerFunc
	AcceptCandidate http.HandlerFunc
	RejectCandidate http.HandlerFunc
}

// PublishRoutes groups handlers for publish-batch review and approval flows.
type PublishRoutes struct {
	ListCandidates    http.HandlerFunc
	CreateBatch       http.HandlerFunc
	GetBatch          http.HandlerFunc
	ReplaceBatchItems http.HandlerFunc
	ReviewApprove     http.HandlerFunc
	ReviewReject      http.HandlerFunc
	ReviewRework      http.HandlerFunc
	OwnerApprove      http.HandlerFunc
	OwnerReject       http.HandlerFunc
	OwnerRework       http.HandlerFunc
	AddBatchFeedback  http.HandlerFunc
	AddItemFeedback   http.HandlerFunc
	GetWorkspace      http.HandlerFunc
	GetRecord         http.HandlerFunc
}

// ArtifactRoutes groups handlers for artifact creation, resolution, and download.
type ArtifactRoutes struct {
	CreatePackage    http.HandlerFunc
	GetArtifact      http.HandlerFunc
	PresignArtifact  http.HandlerFunc
	ResolveArtifact  http.HandlerFunc
	ExportSnapshot   http.HandlerFunc
	CompleteArtifact http.HandlerFunc
	DownloadArtifact http.HandlerFunc
}

// TaskRoutes groups handlers for project-scoped task CRUD endpoints.
type TaskRoutes struct {
	ListTasks      http.HandlerFunc
	CreateTask     http.HandlerFunc
	GetTask        http.HandlerFunc
	TransitionTask http.HandlerFunc
}

// OverviewRoutes groups handlers for the task-first project home payload.
type OverviewRoutes struct {
	GetProjectOverview http.HandlerFunc
}

// Modules collects optional route groups so the server can keep a stable MVP
// route surface while individual modules are delivered incrementally.
type Modules struct {
	DataHub     DataHubRoutes
	Jobs        JobRoutes
	Versioning  VersioningRoutes
	Review      ReviewRoutes
	Publish     PublishRoutes
	Artifacts   ArtifactRoutes
	Tasks       TaskRoutes
	Overview    OverviewRoutes
	ReadyChecks []ReadyCheck
}

// NewHTTPServer builds a control-plane HTTP server with base health routes.
func NewHTTPServer() *HTTPServer {
	return NewHTTPServerWithModules(Modules{})
}

// NewHTTPServerWithDataHub wires only the Data Hub routes for focused setups and tests.
func NewHTTPServerWithDataHub(dataHubHandler *datahub.Handler) *HTTPServer {
	var dataHubRoutes DataHubRoutes
	if dataHubHandler != nil {
		dataHubRoutes = DataHubRoutes{
			CreateDataset:          dataHubHandler.CreateDataset,
			ListDatasets:           dataHubHandler.ListDatasets,
			GetDatasetDetail:       dataHubHandler.GetDatasetDetail,
			GetSnapshotDetail:      dataHubHandler.GetSnapshotDetail,
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

// NewHTTPServerWithDataHubAndJobs wires the Data Hub and Job routes used by the local runtime.
func NewHTTPServerWithDataHubAndJobs(dataHubHandler *datahub.Handler, jobsHandler *jobs.Handler) *HTTPServer {
	var dataHubRoutes DataHubRoutes

	if dataHubHandler != nil {
		dataHubRoutes = DataHubRoutes{
			CreateDataset:          dataHubHandler.CreateDataset,
			ListDatasets:           dataHubHandler.ListDatasets,
			GetDatasetDetail:       dataHubHandler.GetDatasetDetail,
			GetSnapshotDetail:      dataHubHandler.GetSnapshotDetail,
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
// Handlers left unset return 501 so clients can rely on route shape before
// every backing module is fully implemented.
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
		r.Get("/datasets", handlerOrNotImplemented(m.DataHub.ListDatasets))
		r.Get("/datasets/{id}", handlerOrNotImplemented(m.DataHub.GetDatasetDetail))
		r.Post("/datasets/{id}/scan", handlerOrNotImplemented(m.DataHub.ScanDataset))
		r.Post("/datasets/{id}/snapshots", handlerOrNotImplemented(m.DataHub.CreateSnapshot))
		r.Get("/datasets/{id}/snapshots", handlerOrNotImplemented(m.DataHub.ListSnapshots))
		r.Get("/datasets/{id}/items", handlerOrNotImplemented(m.DataHub.ListItems))
		r.Post("/objects/presign", handlerOrNotImplemented(m.DataHub.PresignObject))
		r.Get("/snapshots/{id}", handlerOrNotImplemented(m.DataHub.GetSnapshotDetail))
		r.Post("/snapshots/{id}/import", handlerOrNotImplemented(m.DataHub.ImportSnapshot))
		r.Get("/projects/{id}/overview", handlerOrNotImplemented(m.Overview.GetProjectOverview))
		r.Get("/projects/{id}/tasks", handlerOrNotImplemented(m.Tasks.ListTasks))
		r.Post("/projects/{id}/tasks", handlerOrNotImplemented(m.Tasks.CreateTask))
		r.Get("/tasks/{id}", handlerOrNotImplemented(m.Tasks.GetTask))
		r.Post("/tasks/{id}/transition", handlerOrNotImplemented(m.Tasks.TransitionTask))

		r.Post("/jobs/zero-shot", handlerOrNotImplemented(m.Jobs.CreateZeroShot))
		r.Post("/jobs/video-extract", handlerOrNotImplemented(m.Jobs.CreateVideoExtract))
		r.Post("/jobs/cleaning", handlerOrNotImplemented(m.Jobs.CreateCleaning))
		r.Get("/jobs/{job_id}", handlerOrNotImplemented(m.Jobs.GetJob))
		r.Get("/jobs/{job_id}/events", handlerOrNotImplemented(m.Jobs.ListEvents))

		r.Post("/snapshots/diff", handlerOrNotImplemented(m.Versioning.DiffSnapshots))

		r.Get("/review/candidates", handlerOrNotImplemented(m.Review.ListCandidates))
		r.Post("/review/candidates/{id}/accept", handlerOrNotImplemented(m.Review.AcceptCandidate))
		r.Post("/review/candidates/{id}/reject", handlerOrNotImplemented(m.Review.RejectCandidate))

		r.Get("/publish/candidates", handlerOrNotImplemented(m.Publish.ListCandidates))
		r.Post("/publish/batches", handlerOrNotImplemented(m.Publish.CreateBatch))
		r.Get("/publish/batches/{id}", handlerOrNotImplemented(m.Publish.GetBatch))
		r.Post("/publish/batches/{id}/items", handlerOrNotImplemented(m.Publish.ReplaceBatchItems))
		r.Post("/publish/batches/{id}/review-approve", handlerOrNotImplemented(m.Publish.ReviewApprove))
		r.Post("/publish/batches/{id}/review-reject", handlerOrNotImplemented(m.Publish.ReviewReject))
		r.Post("/publish/batches/{id}/review-rework", handlerOrNotImplemented(m.Publish.ReviewRework))
		r.Post("/publish/batches/{id}/owner-approve", handlerOrNotImplemented(m.Publish.OwnerApprove))
		r.Post("/publish/batches/{id}/owner-reject", handlerOrNotImplemented(m.Publish.OwnerReject))
		r.Post("/publish/batches/{id}/owner-rework", handlerOrNotImplemented(m.Publish.OwnerRework))
		r.Post("/publish/batches/{id}/feedback", handlerOrNotImplemented(m.Publish.AddBatchFeedback))
		r.Post("/publish/batches/{id}/items/{itemId}/feedback", handlerOrNotImplemented(m.Publish.AddItemFeedback))
		r.Get("/publish/batches/{id}/workspace", handlerOrNotImplemented(m.Publish.GetWorkspace))
		r.Get("/publish/records/{id}", handlerOrNotImplemented(m.Publish.GetRecord))

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
		// Keep unimplemented routes visible to clients instead of silently
		// disappearing from the MVP surface.
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}
