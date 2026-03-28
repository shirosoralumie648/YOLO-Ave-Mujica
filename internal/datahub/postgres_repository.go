package datahub

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateDataset(ctx context.Context, in CreateDatasetInput) (Dataset, error) {
	var out Dataset
	err := r.pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id, project_id, name, bucket, prefix
	`, in.ProjectID, in.Name, in.Bucket, in.Prefix).
		Scan(&out.ID, &out.ProjectID, &out.Name, &out.Bucket, &out.Prefix)
	return out, err
}

func (r *PostgresRepository) CreateSnapshot(ctx context.Context, datasetID int64, in CreateSnapshotInput) (Snapshot, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var nextVersion int
	if err := tx.QueryRow(ctx, `
		select coalesce(count(*), 0)::int + 1
		from dataset_snapshots
		where dataset_id = $1
	`, datasetID).Scan(&nextVersion); err != nil {
		return Snapshot{}, err
	}

	version := fmt.Sprintf("v%d", nextVersion)
	var out Snapshot
	err = tx.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, based_on_snapshot_id, created_by, note)
		values ($1, $2, $3, 'system', $4)
		returning id, dataset_id, version, based_on_snapshot_id, note
	`, datasetID, version, in.BasedOnSnapshotID, in.Note).
		Scan(&out.ID, &out.DatasetID, &out.Version, &out.BasedOnSnapshot, &out.Note)
	if err != nil {
		return Snapshot{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Snapshot{}, err
	}
	return out, nil
}

func (r *PostgresRepository) ListSnapshots(ctx context.Context, datasetID int64) ([]Snapshot, error) {
	rows, err := r.pool.Query(ctx, `
		select id, dataset_id, version, based_on_snapshot_id, note
		from dataset_snapshots
		where dataset_id = $1
		order by id asc
	`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := []Snapshot{}
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.DatasetID, &snap.Version, &snap.BasedOnSnapshot, &snap.Note); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, rows.Err()
}

func (r *PostgresRepository) InsertItems(ctx context.Context, datasetID int64, objectKeys []string) (int, error) {
	added := 0
	for _, objectKey := range objectKeys {
		if objectKey == "" {
			continue
		}
		tag, err := r.pool.Exec(ctx, `
			insert into dataset_items (dataset_id, object_key, etag)
			values ($1, $2, md5($2))
			on conflict (dataset_id, object_key) do nothing
		`, datasetID, objectKey)
		if err != nil {
			return 0, err
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}

func (r *PostgresRepository) ListItems(ctx context.Context, datasetID int64) ([]DatasetItem, error) {
	rows, err := r.pool.Query(ctx, `
		select id, dataset_id, object_key, etag
		from dataset_items
		where dataset_id = $1
		order by id asc
	`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []DatasetItem{}
	for rows.Next() {
		var item DatasetItem
		if err := rows.Scan(&item.ID, &item.DatasetID, &item.ObjectKey, &item.ETag); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
