package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"yolo-ave-mujica/internal/annotations"
	"yolo-ave-mujica/internal/artifacts"
	"yolo-ave-mujica/internal/audit"
	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/observability"
	"yolo-ave-mujica/internal/overview"
	"yolo-ave-mujica/internal/publish"
	"yolo-ave-mujica/internal/queue"
	"yolo-ave-mujica/internal/review"
	"yolo-ave-mujica/internal/server"
	"yolo-ave-mujica/internal/storage"
	"yolo-ave-mujica/internal/store"
	"yolo-ave-mujica/internal/tasks"
	"yolo-ave-mujica/internal/versioning"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	modules, cleanup, sweepTick, err := buildModules(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	startBackgroundLoop(ctx, cfg.SweeperInterval, sweepTick)

	if err := run(ctx, cfg, modules); err != nil {
		log.Fatal(err)
	}
}

// buildModules wires the control-plane modules around the shared runtime dependencies.
func buildModules(ctx context.Context, cfg config.Config) (server.Modules, func(), func(time.Time) error, error) {
	// Build shared infrastructure first because every domain module depends on
	// PostgreSQL, Redis, or object storage clients created here.
	pool, err := store.NewPostgresPool(ctx, cfg)
	if err != nil {
		return server.Modules{}, nil, nil, err
	}

	s3Client, err := storage.NewS3Client(cfg)
	if err != nil {
		pool.Close()
		return server.Modules{}, nil, nil, err
	}

	dataHubRepo := datahub.NewPostgresRepository(pool)
	dataHubSvc := datahub.NewServiceWithRepositoryAndScanner(
		func(datasetID int64, objectKey string, ttlSeconds int) (string, error) {
			return storage.PresignURLString(s3Client, cfg.S3Bucket, objectKey, time.Duration(ttlSeconds)*time.Second)
		},
		dataHubRepo,
		s3ObjectScanner{client: s3Client},
	)

	redisClient := queue.NewRedisClient(cfg)
	jobsRepo := jobs.NewPostgresRepository(pool)
	reviewRepo := review.NewPostgresRepository(pool)
	reviewSvc := review.NewServiceWithRepository(reviewRepo)
	metrics := observability.NewMetrics()
	metrics.SetQueueDepthProvider(func(ctx context.Context) (map[string]int64, error) {
		depths := map[string]int64{}
		for _, lane := range []string{"jobs:cpu", "jobs:gpu", "jobs:mixed"} {
			value, err := redisClient.LLen(ctx, lane).Result()
			if err != nil {
				return nil, err
			}
			depths[lane] = value
		}
		return depths, nil
	})
	metrics.SetReviewBacklogProvider(func(context.Context) (int64, error) {
		count, err := reviewRepo.PendingCandidateCount(1)
		return int64(count), err
	})
	httpLogger := observability.NewJSONLogger(log.Writer())
	auditLogger := audit.NewPostgresLogger(pool)
	jobsSvc := jobs.NewServiceWithDependenciesAndMetrics(jobsRepo, jobs.NewRedisPublisher(redisClient), reviewCandidateSinkAdapter{svc: reviewSvc}, metrics)
	jobsHandler := jobs.NewHandlerWithAudit(jobsSvc, auditLogger)
	dataHubHandler := datahub.NewHandlerWithJobsAndSourcePresignAndAudit(dataHubSvc, jobsSvc, func(sourceURI string, ttlSeconds int) (string, error) {
		return storage.PresignURI(s3Client, sourceURI, time.Duration(ttlSeconds)*time.Second)
	}, auditLogger)
	jobSweeper := jobs.NewSweeperWithMetrics(jobsRepo, jobs.NewRedisPublisher(redisClient), 3, metrics).
		WithRetryBackoff(cfg.JobRetryBackoffBase, cfg.JobRetryBackoffMax)

	versioningHandler := versioning.NewHandler(versioning.NewServiceWithRepository(versioning.NewPostgresRepository(pool)))
	reviewHandler := review.NewHandler(reviewSvc)
	taskRepo := tasks.NewPostgresRepository(pool)
	taskSvc := tasks.NewService(taskRepo)
	taskHandler := tasks.NewHandler(taskSvc)
	annotationRepo := annotations.NewPostgresRepository(pool)
	annotationSvc := annotations.NewServiceWithTaskService(annotationRepo, taskSvc)
	annotationHandler := annotations.NewHandler(annotationSvc)
	publishRepo := publish.NewPostgresRepository(pool)
	publishSvc := publish.NewService(publishRepo, taskSvc)
	publishHandler := publish.NewHandlerWithAudit(publishSvc, auditLogger)
	overviewSvc := overview.NewService(
		overview.TaskSourceFunc(func(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error) {
			return taskSvc.ListTasks(context.Background(), projectID, filter)
		}),
		reviewRepo,
		jobsRepo,
	)
	overviewHandler := overview.NewHandler(overviewSvc)

	artifactRepo := artifacts.NewPostgresRepository(pool)
	artifactQuery := artifacts.NewExportQuery(pool)
	artifactStorage := artifacts.NewFilesystemStorage(cfg.ArtifactStorageDir)
	artifactSource := artifacts.StreamingObjectSourceFunc(func(ctx context.Context, objectKey string) (io.ReadCloser, error) {
		obj, err := s3Client.GetObject(ctx, cfg.S3Bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			return nil, err
		}
		return obj, nil
	})
	artifactBuilder := artifacts.NewBuilder(artifactSource)
	artifactService := artifacts.NewServiceWithDependencies(artifactRepo, artifactQuery, artifactBuilder, artifactStorage)
	artifactService = artifactService.WithMetrics(metrics)
	// Recover unfinished builds before serving traffic so stale "building"
	// rows do not survive an unclean restart indefinitely.
	if _, err := artifactService.MarkStaleBuildsFailed(ctx, "startup_recovery"); err != nil {
		_ = redisClient.Close()
		pool.Close()
		return server.Modules{}, nil, nil, err
	}
	// Start the in-process build runner only after startup recovery has
	// finished so newly queued work starts from a known-good baseline.
	artifactService.StartBuildRunner(ctx, cfg.ArtifactBuildConcurrency)
	artifactHandler := artifacts.NewHandlerWithJobsAndAudit(artifactService, nil, auditLogger)

	modules := buildModulesWithHandlers(reviewHandler, publishHandler, artifactHandler)
	modules.DataHub = server.DataHubRoutes{
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
	modules.Jobs = server.JobRoutes{
		CreateZeroShot:     jobsHandler.CreateZeroShot,
		CreateVideoExtract: jobsHandler.CreateVideoExtract,
		CreateCleaning:     jobsHandler.CreateCleaning,
		GetJob:             jobsHandler.GetJob,
		ListWorkers:        jobsHandler.ListWorkers,
		ListEvents:         jobsHandler.ListEvents,
		RegisterWorker:     jobsHandler.RegisterWorker,
		ReportHeartbeat:    jobsHandler.ReportHeartbeat,
		ReportProgress:     jobsHandler.ReportProgress,
		ReportItemError:    jobsHandler.ReportItemError,
		ReportTerminal:     jobsHandler.ReportTerminal,
	}
	modules.Versioning = server.VersioningRoutes{
		DiffSnapshots: versioningHandler.DiffSnapshots,
	}
	modules.Tasks = server.TaskRoutes{
		ListTasks:      taskHandler.ListTasks,
		CreateTask:     taskHandler.CreateTask,
		GetTask:        taskHandler.GetTask,
		TransitionTask: taskHandler.TransitionTask,
	}
	modules.Annotations = server.AnnotationRoutes{
		GetWorkspace:    annotationHandler.GetWorkspace,
		SaveDraft:       annotationHandler.SaveDraft,
		SubmitWorkspace: annotationHandler.SubmitWorkspace,
	}
	modules.Overview = server.OverviewRoutes{
		GetProjectOverview: overviewHandler.GetProjectOverview,
	}
	modules.ReadyChecks = []server.ReadyCheck{
		func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				return fmt.Errorf("postgres not ready: %w", err)
			}
			return nil
		},
		func(ctx context.Context) error {
			if err := queue.Ping(ctx, redisClient); err != nil {
				return fmt.Errorf("redis not ready: %w", err)
			}
			return nil
		},
		func(ctx context.Context) error {
			if _, err := s3Client.ListBuckets(ctx); err != nil {
				return fmt.Errorf("s3 not ready: %w", err)
			}
			return nil
		},
		func(_ context.Context) error {
			if cfg.S3Bucket == "" {
				return fmt.Errorf("s3 bucket is not configured")
			}
			return nil
		},
	}
	modules.HTTPMiddleware = func(next http.Handler) http.Handler {
		return auth.IdentityMiddleware(cfg.AuthDefaultProjectIDs)(
			observability.Middleware(httpLogger, metrics)(next),
		)
	}
	modules.MetricsHandler = metrics.Handler()
	modules.MutationMiddleware = func(next http.Handler) http.Handler {
		return auth.StaticBearerMiddleware(cfg.AuthBearerToken)(
			auth.FixedWindowRateLimitMiddleware(cfg.MutationRateLimitPerMinute, time.Minute)(
				next,
			),
		)
	}

	return modules, func() {
		_ = redisClient.Close()
		pool.Close()
	}, jobSweeper.Tick, nil
}

type s3ObjectScanner struct {
	client *minio.Client
}

type reviewCandidateSinkAdapter struct {
	svc *review.Service
}

func (a reviewCandidateSinkAdapter) PersistCandidates(jobID int64, items []jobs.ReviewCandidateInput) ([]jobs.PersistedReviewCandidate, error) {
	inputs := make([]review.PersistCandidateInput, 0, len(items))
	for _, item := range items {
		inputs = append(inputs, review.PersistCandidateInput{
			DatasetID:    item.DatasetID,
			SnapshotID:   item.SnapshotID,
			ItemID:       item.ItemID,
			ObjectKey:    item.ObjectKey,
			CategoryID:   item.CategoryID,
			CategoryName: item.CategoryName,
			BBox: review.CandidateBBox{
				X: item.BBox.X,
				Y: item.BBox.Y,
				W: item.BBox.W,
				H: item.BBox.H,
			},
			Confidence: item.Confidence,
			ModelName:  item.ModelName,
			IsPseudo:   item.IsPseudo,
		})
	}
	persisted, err := a.svc.PersistCandidates(jobID, inputs)
	if err != nil {
		return nil, err
	}
	out := make([]jobs.PersistedReviewCandidate, 0, len(persisted))
	for _, candidate := range persisted {
		out = append(out, jobs.PersistedReviewCandidate{ID: candidate.ID})
	}
	return out, nil
}

func (s s3ObjectScanner) ListObjects(bucket, prefix string) ([]datahub.ScannedObject, error) {
	objects, err := storage.ListObjects(s.client, bucket, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]datahub.ScannedObject, 0, len(objects))
	for _, object := range objects {
		out = append(out, datahub.ScannedObject{
			Key:  object.Key,
			ETag: object.ETag,
			Size: object.Size,
		})
	}
	return out, nil
}

func buildModulesWithHandlers(reviewHandler *review.Handler, publishHandler *publish.Handler, artifactHandler *artifacts.Handler) server.Modules {
	modules := server.Modules{}
	if reviewHandler != nil {
		modules.Review = server.ReviewRoutes{
			ListCandidates:  reviewHandler.ListCandidates,
			AcceptCandidate: reviewHandler.AcceptCandidate,
			RejectCandidate: reviewHandler.RejectCandidate,
		}
	}
	if publishHandler != nil {
		modules.Publish = server.PublishRoutes{
			ListCandidates:    publishHandler.ListSuggestedCandidates,
			CreateBatch:       publishHandler.CreateBatch,
			GetBatch:          publishHandler.GetBatch,
			ReplaceBatchItems: publishHandler.ReplaceBatchItems,
			ReviewApprove:     publishHandler.ReviewApprove,
			ReviewReject:      publishHandler.ReviewReject,
			ReviewRework:      publishHandler.ReviewRework,
			OwnerApprove:      publishHandler.OwnerApprove,
			OwnerReject:       publishHandler.OwnerReject,
			OwnerRework:       publishHandler.OwnerRework,
			AddBatchFeedback:  publishHandler.AddBatchFeedback,
			AddItemFeedback:   publishHandler.AddItemFeedback,
			GetWorkspace:      publishHandler.GetWorkspace,
			GetRecord:         publishHandler.GetRecord,
		}
	}
	if artifactHandler != nil {
		modules.Artifacts = server.ArtifactRoutes{
			CreatePackage:    artifactHandler.CreatePackage,
			ExportSnapshot:   artifactHandler.ExportSnapshot,
			GetArtifact:      artifactHandler.GetArtifact,
			PresignArtifact:  artifactHandler.PresignArtifact,
			ResolveArtifact:  artifactHandler.ResolveArtifact,
			CompleteArtifact: artifactHandler.CompleteArtifact,
			DownloadArtifact: artifactHandler.DownloadArtifact,
		}
	}
	return modules
}

// startBackgroundLoop runs a lightweight periodic callback until shutdown.
func startBackgroundLoop(ctx context.Context, interval time.Duration, tick func(time.Time) error) {
	if tick == nil || interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				// The shared loop currently drives job sweeping, but keeping it
				// generic makes future periodic runtime tasks easy to add.
				if err := tick(now); err != nil {
					log.Printf("background loop tick failed: %v", err)
				}
			}
		}
	}()
}

func run(ctx context.Context, cfg config.Config, modules server.Modules) error {
	if err := ctx.Err(); err != nil {
		return nil
	}

	httpServer := server.NewHTTPServerWithModules(modules)
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpServer.Handler,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}
