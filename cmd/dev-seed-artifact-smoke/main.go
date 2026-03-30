package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"yolo-ave-mujica/internal/config"
	appstorage "yolo-ave-mujica/internal/storage"
	"yolo-ave-mujica/internal/store"
)

type seedResult struct {
	DatasetID  int64  `json:"dataset_id"`
	SnapshotID int64  `json:"snapshot_id"`
	Version    string `json:"version"`
}

func main() {
	datasetID := flag.Int64("dataset-id", 0, "existing dataset id to seed")
	projectID := flag.Int64("project-id", 1, "project id")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	s3Client, err := appstorage.NewS3Client(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := appstorage.EnsureBucket(ctx, s3Client, cfg.S3Bucket); err != nil {
		log.Fatal(err)
	}

	result, err := seedArtifactSmokeData(ctx, pool, s3Client, cfg.S3Bucket, *datasetID, *projectID)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Fatal(err)
	}
}

func seedArtifactSmokeData(ctx context.Context, pool *pgxpool.Pool, s3Client *minio.Client, bucket string, datasetID, projectID int64) (seedResult, error) {
	if datasetID == 0 {
		createdID, createdProjectID, err := ensureDataset(ctx, pool, projectID)
		if err != nil {
			return seedResult{}, err
		}
		datasetID = createdID
		projectID = createdProjectID
	} else {
		var actualProjectID int64
		if err := pool.QueryRow(ctx, `select project_id from datasets where id = $1`, datasetID).Scan(&actualProjectID); err != nil {
			return seedResult{}, fmt.Errorf("load dataset %d: %w", datasetID, err)
		}
		projectID = actualProjectID
	}

	itemIDs, err := ensureDatasetItems(ctx, pool, datasetID, []string{"train/a.jpg", "train/b.jpg"})
	if err != nil {
		return seedResult{}, err
	}
	snapshotID, version, err := ensureSnapshot(ctx, pool, datasetID)
	if err != nil {
		return seedResult{}, err
	}
	categoryID, err := ensureCategory(ctx, pool, projectID, "person")
	if err != nil {
		return seedResult{}, err
	}
	if err := ensureAnnotation(ctx, pool, datasetID, itemIDs["train/a.jpg"], categoryID, snapshotID); err != nil {
		return seedResult{}, err
	}
	if err := uploadFixtureImages(ctx, s3Client, bucket); err != nil {
		return seedResult{}, err
	}

	return seedResult{
		DatasetID:  datasetID,
		SnapshotID: snapshotID,
		Version:    version,
	}, nil
}

func ensureDataset(ctx context.Context, pool *pgxpool.Pool, projectID int64) (int64, int64, error) {
	var datasetID int64
	var actualProjectID int64
	err := pool.QueryRow(ctx, `
		select id, project_id
		from datasets
		where project_id = $1 and name = 'artifact-smoke-dataset'
		order by id asc
		limit 1
	`, projectID).Scan(&datasetID, &actualProjectID)
	if err == nil {
		return datasetID, actualProjectID, nil
	}

	err = pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, 'artifact-smoke-dataset', 'platform-dev', 'train')
		returning id, project_id
	`, projectID).Scan(&datasetID, &actualProjectID)
	if err != nil {
		return 0, 0, err
	}
	return datasetID, actualProjectID, nil
}

func ensureDatasetItems(ctx context.Context, pool *pgxpool.Pool, datasetID int64, objectKeys []string) (map[string]int64, error) {
	itemIDs := make(map[string]int64, len(objectKeys))
	for _, objectKey := range objectKeys {
		if _, err := pool.Exec(ctx, `
			insert into dataset_items (dataset_id, object_key, etag)
			values ($1, $2, md5($2))
			on conflict (dataset_id, object_key) do nothing
		`, datasetID, objectKey); err != nil {
			return nil, err
		}

		var itemID int64
		if err := pool.QueryRow(ctx, `
			select id
			from dataset_items
			where dataset_id = $1 and object_key = $2
		`, datasetID, objectKey).Scan(&itemID); err != nil {
			return nil, err
		}
		itemIDs[objectKey] = itemID
	}
	return itemIDs, nil
}

func ensureSnapshot(ctx context.Context, pool *pgxpool.Pool, datasetID int64) (int64, string, error) {
	var snapshotID int64
	var version string
	err := pool.QueryRow(ctx, `
		select id, version
		from dataset_snapshots
		where dataset_id = $1
		order by id asc
		limit 1
	`, datasetID).Scan(&snapshotID, &version)
	if err == nil {
		return snapshotID, version, nil
	}

	err = pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, 'v1', 'smoke', 'artifact smoke seed')
		returning id, version
	`, datasetID).Scan(&snapshotID, &version)
	if err != nil {
		return 0, "", err
	}
	return snapshotID, version, nil
}

func ensureCategory(ctx context.Context, pool *pgxpool.Pool, projectID int64, name string) (int64, error) {
	var categoryID int64
	err := pool.QueryRow(ctx, `
		insert into categories (project_id, name)
		values ($1, $2)
		on conflict (project_id, name) do update set name = excluded.name
		returning id
	`, projectID, name).Scan(&categoryID)
	if err == nil {
		return categoryID, nil
	}

	if err := pool.QueryRow(ctx, `
		select id
		from categories
		where project_id = $1 and name = $2
	`, projectID, name).Scan(&categoryID); err != nil {
		return 0, err
	}
	return categoryID, nil
}

func ensureAnnotation(ctx context.Context, pool *pgxpool.Pool, datasetID, itemID, categoryID, snapshotID int64) error {
	var existing int64
	err := pool.QueryRow(ctx, `
		select id
		from annotations
		where dataset_id = $1
		  and item_id = $2
		  and category_id = $3
		  and created_at_snapshot_id = $4
		  and bbox_x = 0.4
		  and bbox_y = 0.4
		  and bbox_w = 0.2
		  and bbox_h = 0.2
		limit 1
	`, datasetID, itemID, categoryID, snapshotID).Scan(&existing)
	if err == nil {
		return nil
	}

	_, err = pool.Exec(ctx, `
		insert into annotations (
			dataset_id, item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h,
			created_at_snapshot_id, review_status, is_pseudo
		)
		values ($1, $2, $3, 0.4, 0.4, 0.2, 0.2, $4, 'verified', false)
	`, datasetID, itemID, categoryID, snapshotID)
	return err
}

func uploadFixtureImages(ctx context.Context, s3Client *minio.Client, bucket string) error {
	images := map[string]color.Color{
		"train/a.jpg": color.RGBA{R: 220, G: 80, B: 80, A: 255},
		"train/b.jpg": color.RGBA{R: 80, G: 140, B: 220, A: 255},
	}
	for objectKey, fill := range images {
		body, err := generateJPEG(fill)
		if err != nil {
			return err
		}
		_, err = s3Client.PutObject(ctx, bucket, objectKey, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
			ContentType: "image/jpeg",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func generateJPEG(fill color.Color) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, fill)
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
