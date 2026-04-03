package datahub

import (
	"context"
	"database/sql"
	"errors"
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

func (r *PostgresRepository) GetDataset(ctx context.Context, datasetID int64) (Dataset, error) {
	var out Dataset
	err := r.pool.QueryRow(ctx, `
		select id, project_id, name, bucket, prefix
		from datasets
		where id = $1
	`, datasetID).Scan(&out.ID, &out.ProjectID, &out.Name, &out.Bucket, &out.Prefix)
	return out, err
}

func (r *PostgresRepository) ListDatasets(ctx context.Context, projectID int64) ([]DatasetSummary, error) {
	rows, err := r.pool.Query(ctx, `
		select
			d.id,
			d.project_id,
			d.name,
			d.bucket,
			d.prefix,
			(select count(*)::int from dataset_items i where i.dataset_id = d.id) as item_count,
			(select count(*)::int from dataset_snapshots s where s.dataset_id = d.id) as snapshot_count,
			(select s.id from dataset_snapshots s where s.dataset_id = d.id order by s.id desc limit 1) as latest_snapshot_id,
			(select s.version from dataset_snapshots s where s.dataset_id = d.id order by s.id desc limit 1) as latest_snapshot_version
		from datasets d
		where d.project_id = $1
		order by d.id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []DatasetSummary{}
	for rows.Next() {
		var item DatasetSummary
		var latestSnapshotID sql.NullInt64
		var latestSnapshotVersion sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.Name,
			&item.Bucket,
			&item.Prefix,
			&item.ItemCount,
			&item.SnapshotCount,
			&latestSnapshotID,
			&latestSnapshotVersion,
		); err != nil {
			return nil, err
		}
		if latestSnapshotID.Valid {
			id := latestSnapshotID.Int64
			item.LatestSnapshotID = &id
		}
		if latestSnapshotVersion.Valid {
			item.LatestSnapshotVersion = latestSnapshotVersion.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) GetDatasetDetail(ctx context.Context, datasetID int64) (DatasetDetail, error) {
	var out DatasetDetail
	var latestSnapshotID sql.NullInt64
	var latestSnapshotVersion sql.NullString
	err := r.pool.QueryRow(ctx, `
		select
			d.id,
			d.project_id,
			d.name,
			d.bucket,
			d.prefix,
			(select count(*)::int from dataset_items i where i.dataset_id = d.id) as item_count,
			(select count(*)::int from dataset_snapshots s where s.dataset_id = d.id) as snapshot_count,
			(select s.id from dataset_snapshots s where s.dataset_id = d.id order by s.id desc limit 1) as latest_snapshot_id,
			(select s.version from dataset_snapshots s where s.dataset_id = d.id order by s.id desc limit 1) as latest_snapshot_version
		from datasets d
		where d.id = $1
	`, datasetID).Scan(
		&out.ID,
		&out.ProjectID,
		&out.Name,
		&out.Bucket,
		&out.Prefix,
		&out.ItemCount,
		&out.SnapshotCount,
		&latestSnapshotID,
		&latestSnapshotVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DatasetDetail{}, wrapNotFound("dataset", datasetID)
	}
	if latestSnapshotID.Valid {
		id := latestSnapshotID.Int64
		out.LatestSnapshotID = &id
	}
	if latestSnapshotVersion.Valid {
		out.LatestSnapshotVersion = latestSnapshotVersion.String
	}
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

func (r *PostgresRepository) GetSnapshot(ctx context.Context, snapshotID int64) (Snapshot, error) {
	var out Snapshot
	err := r.pool.QueryRow(ctx, `
		select id, dataset_id, version, based_on_snapshot_id, coalesce(note, '')
		from dataset_snapshots
		where id = $1
	`, snapshotID).Scan(&out.ID, &out.DatasetID, &out.Version, &out.BasedOnSnapshot, &out.Note)
	return out, err
}

func (r *PostgresRepository) GetSnapshotDetail(ctx context.Context, snapshotID int64) (SnapshotDetail, error) {
	var out SnapshotDetail
	err := r.pool.QueryRow(ctx, `
		select
			s.id,
			s.dataset_id,
			s.version,
			s.based_on_snapshot_id,
			coalesce(s.note, ''),
				d.project_id,
				d.name,
				(select count(*)::int
					from annotations a
					where a.dataset_id = s.dataset_id
						and a.created_at_snapshot_id <= s.id
						and (a.deleted_at_snapshot_id is null or a.deleted_at_snapshot_id > s.id)
						and a.review_status = 'verified'
				) as annotation_count
			from dataset_snapshots s
		join datasets d on d.id = s.dataset_id
		where s.id = $1
	`, snapshotID).Scan(
		&out.ID,
		&out.DatasetID,
		&out.Version,
		&out.BasedOnSnapshotID,
		&out.Note,
		&out.ProjectID,
		&out.DatasetName,
		&out.AnnotationCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SnapshotDetail{}, wrapNotFound("snapshot", snapshotID)
	}
	return out, err
}

func (r *PostgresRepository) ListSnapshots(ctx context.Context, datasetID int64) ([]Snapshot, error) {
	rows, err := r.pool.Query(ctx, `
		select id, dataset_id, version, based_on_snapshot_id, coalesce(note, '')
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
	objects := make([]ScannedObject, 0, len(objectKeys))
	for _, objectKey := range objectKeys {
		if objectKey == "" {
			continue
		}
		objects = append(objects, ScannedObject{Key: objectKey})
	}
	return r.UpsertScannedItems(ctx, datasetID, objects)
}

func (r *PostgresRepository) UpsertScannedItems(ctx context.Context, datasetID int64, objects []ScannedObject) (int, error) {
	added := 0
	for _, object := range objects {
		if object.Key == "" {
			continue
		}
		tag, err := r.pool.Exec(ctx, `
			insert into dataset_items (dataset_id, object_key, etag, size, width, height, mime)
			values ($1, $2, nullif($3, ''), $4, nullif($5, 0), nullif($6, 0), nullif($7, ''))
			on conflict (dataset_id, object_key) do update set
				etag = coalesce(excluded.etag, dataset_items.etag),
				size = coalesce(excluded.size, dataset_items.size),
				width = coalesce(excluded.width, dataset_items.width),
				height = coalesce(excluded.height, dataset_items.height),
				mime = coalesce(excluded.mime, dataset_items.mime)
		`, datasetID, object.Key, object.ETag, object.Size, object.Width, object.Height, object.Mime)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() > 0 {
			added++
		}
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
		var etag sql.NullString
		if err := rows.Scan(&item.ID, &item.DatasetID, &item.ObjectKey, &etag); err != nil {
			return nil, err
		}
		item.ETag = etag.String
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) GetItemByObjectKey(ctx context.Context, datasetID int64, objectKey string) (DatasetItem, error) {
	var item DatasetItem
	var etag sql.NullString
	err := r.pool.QueryRow(ctx, `
		select id, dataset_id, object_key, etag
		from dataset_items
		where dataset_id = $1 and object_key = $2
	`, datasetID, objectKey).Scan(&item.ID, &item.DatasetID, &item.ObjectKey, &etag)
	if err != nil {
		return DatasetItem{}, err
	}
	item.ETag = etag.String
	return item, nil
}

func (r *PostgresRepository) EnsureCategory(ctx context.Context, projectID int64, categoryName string) (int64, error) {
	var categoryID int64
	err := r.pool.QueryRow(ctx, `
		insert into categories (project_id, name)
		values ($1, $2)
		on conflict (project_id, name) do update set
			name = excluded.name
		returning id
	`, projectID, categoryName).Scan(&categoryID)
	return categoryID, err
}

func (r *PostgresRepository) CreateAnnotation(ctx context.Context, snapshotID, datasetID, itemID int64, _ string, categoryID int64, _ string, bboxX, bboxY, bboxW, bboxH float64) error {
	_, err := r.pool.Exec(ctx, `
		insert into annotations (
			dataset_id, item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h,
			source, created_at_snapshot_id, review_status, is_pseudo
		)
		values ($1, $2, $3, $4, $5, $6, $7, 'import', $8, 'verified', false)
	`, datasetID, itemID, categoryID, bboxX, bboxY, bboxW, bboxH, snapshotID)
	return err
}
