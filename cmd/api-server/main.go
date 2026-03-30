package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"yolo-ave-mujica/internal/artifacts"
	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/queue"
	"yolo-ave-mujica/internal/review"
	"yolo-ave-mujica/internal/server"
	"yolo-ave-mujica/internal/storage"
	"yolo-ave-mujica/internal/store"
	"yolo-ave-mujica/internal/versioning"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	modules, cleanup, sweepTick, err := buildModules(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	startBackgroundLoop(ctx, cfg.SweeperInterval, sweepTick)

	if err := run(ctx, cfg, modules); err != nil {
		log.Fatal(err)
	}
}

func buildModules(cfg config.Config) (server.Modules, func(), func(time.Time) error, error) {
	ctx := context.Background()
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
	jobsSvc := jobs.NewServiceWithPublisher(jobsRepo, jobs.NewRedisPublisher(redisClient))
	jobsHandler := jobs.NewHandler(jobsSvc)
	dataHubHandler := datahub.NewHandlerWithJobsAndSourcePresign(dataHubSvc, jobsSvc, func(sourceURI string, ttlSeconds int) (string, error) {
		return storage.PresignURI(s3Client, sourceURI, time.Duration(ttlSeconds)*time.Second)
	})
	jobSweeper := jobs.NewSweeper(jobsRepo, jobs.NewRedisPublisher(redisClient), 3)

	versioningHandler := versioning.NewHandler(versioning.NewServiceWithRepository(versioning.NewPostgresRepository(pool)))
	reviewRepo := review.NewPostgresRepository(pool)
	reviewHandler := review.NewHandler(review.NewServiceWithRepository(reviewRepo))
	artifactRepo := artifacts.NewPostgresRepository(pool)
	artifactSvc := artifacts.NewServiceWithRepositoryAndStorageAndBucket(
		artifactRepo,
		cfg.S3Bucket,
		func(uri string, body []byte, contentType string) (int64, error) {
			return storage.UploadURI(s3Client, uri, body, contentType)
		},
		func(uri string, ttlSeconds int) (string, error) {
			return storage.PresignURI(s3Client, uri, time.Duration(ttlSeconds)*time.Second)
		},
	)
	artifactHandler := artifacts.NewHandlerWithJobs(artifactSvc, jobsSvc)

	modules := buildModulesWithHandlers(reviewHandler, artifactHandler)
	modules.DataHub = server.DataHubRoutes{
		CreateDataset:          dataHubHandler.CreateDataset,
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
		ListEvents:         jobsHandler.ListEvents,
		ReportHeartbeat:    jobsHandler.ReportHeartbeat,
		ReportProgress:     jobsHandler.ReportProgress,
		ReportItemError:    jobsHandler.ReportItemError,
		ReportTerminal:     jobsHandler.ReportTerminal,
	}
	modules.Versioning = server.VersioningRoutes{
		DiffSnapshots: versioningHandler.DiffSnapshots,
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

	return modules, func() {
		_ = redisClient.Close()
		pool.Close()
	}, jobSweeper.Tick, nil
}

type s3ObjectScanner struct {
	client *minio.Client
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

func buildModulesWithHandlers(reviewHandler *review.Handler, artifactHandler *artifacts.Handler) server.Modules {
	modules := server.Modules{}
	if reviewHandler != nil {
		modules.Review = server.ReviewRoutes{
			ListCandidates:  reviewHandler.ListCandidates,
			AcceptCandidate: reviewHandler.AcceptCandidate,
			RejectCandidate: reviewHandler.RejectCandidate,
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
		}
	}
	return modules
}

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
