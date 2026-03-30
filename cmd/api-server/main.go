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

func buildModules(ctx context.Context, cfg config.Config) (server.Modules, func(), func(time.Time) error, error) {
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
	dataHubSvc := datahub.NewServiceWithRepository(func(datasetID int64, objectKey string, ttlSeconds int) (string, error) {
		return storage.PresignURLString(s3Client, cfg.S3Bucket, objectKey, time.Duration(ttlSeconds)*time.Second)
	}, dataHubRepo)
	dataHubHandler := datahub.NewHandler(dataHubSvc)

	redisClient := queue.NewRedisClient(cfg)
	jobsRepo := jobs.NewPostgresRepository(pool)
	jobsSvc := jobs.NewServiceWithPublisher(jobsRepo, jobs.NewRedisPublisher(redisClient))
	jobsHandler := jobs.NewHandler(jobsSvc)
	jobSweeper := jobs.NewSweeper(jobsRepo, jobs.NewRedisPublisher(redisClient), 3)

	versioningHandler := versioning.NewHandler(versioning.NewService())
	reviewHandler := review.NewHandler(review.NewService())
	artifactRepo := artifacts.NewPostgresRepository(pool)
	artifactQuery := artifacts.NewExportQuery(pool)
	artifactStorage := artifacts.NewFilesystemStorage(cfg.ArtifactStorageDir)
	artifactSource := artifacts.ObjectSourceFunc(func(ctx context.Context, objectKey string) ([]byte, error) {
		obj, err := s3Client.GetObject(ctx, cfg.S3Bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			return nil, err
		}
		defer obj.Close()
		return io.ReadAll(obj)
	})
	artifactBuilder := artifacts.NewBuilder(artifactSource)
	artifactService := artifacts.NewServiceWithDependencies(artifactRepo, artifactQuery, artifactBuilder, artifactStorage)
	if _, err := artifactService.MarkStaleBuildsFailed(ctx, "startup_recovery"); err != nil {
		_ = redisClient.Close()
		pool.Close()
		return server.Modules{}, nil, nil, err
	}
	artifactService.StartBuildRunner(ctx, cfg.ArtifactBuildConcurrency)
	artifactHandler := artifacts.NewHandler(artifactService)

	modules := server.Modules{
		DataHub: server.DataHubRoutes{
			CreateDataset:  dataHubHandler.CreateDataset,
			ScanDataset:    dataHubHandler.ScanDataset,
			CreateSnapshot: dataHubHandler.CreateSnapshot,
			ListSnapshots:  dataHubHandler.ListSnapshots,
			ListItems:      dataHubHandler.ListItems,
			PresignObject:  dataHubHandler.PresignObject,
		},
		Jobs: server.JobRoutes{
			CreateZeroShot:     jobsHandler.CreateZeroShot,
			CreateVideoExtract: jobsHandler.CreateVideoExtract,
			CreateCleaning:     jobsHandler.CreateCleaning,
			GetJob:             jobsHandler.GetJob,
			ListEvents:         jobsHandler.ListEvents,
		},
		Versioning: server.VersioningRoutes{
			DiffSnapshots: versioningHandler.DiffSnapshots,
		},
		Review: server.ReviewRoutes{
			ListCandidates:  reviewHandler.ListCandidates,
			AcceptCandidate: reviewHandler.AcceptCandidate,
			RejectCandidate: reviewHandler.RejectCandidate,
		},
		Artifacts: server.ArtifactRoutes{
			CreatePackage:    artifactHandler.CreatePackage,
			GetArtifact:      artifactHandler.GetArtifact,
			PresignArtifact:  artifactHandler.PresignArtifact,
			ResolveArtifact:  artifactHandler.ResolveArtifact,
			DownloadArtifact: artifactHandler.DownloadArtifact,
		},
		ReadyChecks: []server.ReadyCheck{
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
		},
	}

	return modules, func() {
		_ = redisClient.Close()
		pool.Close()
	}, jobSweeper.Tick, nil
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
