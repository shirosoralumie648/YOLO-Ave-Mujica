package artifacts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, a Artifact) (Artifact, error) {
	labelMapJSON, err := json.Marshal(a.LabelMapJSON)
	if err != nil {
		return Artifact{}, err
	}

	row := r.pool.QueryRow(ctx, `
		insert into artifacts (
			project_id, dataset_id, snapshot_id, artifact_type, format, version,
			uri, checksum, size, manifest_uri, label_map_json, status, error_msg, created_by_job_id
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		returning id, project_id, dataset_id, snapshot_id, artifact_type, format, version,
		          created_by_job_id, uri, manifest_uri, checksum, size, label_map_json, status, error_msg, created_at
	`,
		a.ProjectID,
		a.DatasetID,
		a.SnapshotID,
		a.ArtifactType,
		a.Format,
		a.Version,
		a.URI,
		a.Checksum,
		a.Size,
		a.ManifestURI,
		labelMapJSON,
		a.Status,
		a.ErrorMsg,
		a.CreatedByJobID,
	)
	return scanArtifact(row)
}

func (r *PostgresRepository) Get(ctx context.Context, id int64) (Artifact, bool, error) {
	row := r.pool.QueryRow(ctx, `
		select id, project_id, dataset_id, snapshot_id, artifact_type, format, version,
		       created_by_job_id, uri, manifest_uri, checksum, size, label_map_json, status, error_msg, created_at
		from artifacts
		where id = $1
	`, id)

	artifact, err := scanArtifact(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Artifact{}, false, nil
		}
		return Artifact{}, false, err
	}
	return artifact, true, nil
}

func (r *PostgresRepository) FindReadyByFormatVersion(ctx context.Context, format, version string) (Artifact, bool, error) {
	row := r.pool.QueryRow(ctx, `
		select id, project_id, dataset_id, snapshot_id, artifact_type, format, version,
		       created_by_job_id, uri, manifest_uri, checksum, size, label_map_json, status, error_msg, created_at
		from artifacts
		where format = $1 and version = $2 and status = $3
		order by id desc
		limit 1
	`, format, version, StatusReady)

	artifact, err := scanArtifact(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Artifact{}, false, nil
		}
		return Artifact{}, false, err
	}
	return artifact, true, nil
}

func (r *PostgresRepository) FindReadyByDatasetFormatVersion(ctx context.Context, dataset, format, version string) (Artifact, bool, error) {
	row := r.pool.QueryRow(ctx, `
		select artifacts.id, artifacts.project_id, artifacts.dataset_id, artifacts.snapshot_id,
		       artifacts.artifact_type, artifacts.format, artifacts.version, artifacts.created_by_job_id, artifacts.uri,
		       artifacts.manifest_uri, artifacts.checksum, artifacts.size, artifacts.label_map_json,
		       artifacts.status, artifacts.error_msg, artifacts.created_at
		from artifacts
		left join datasets on datasets.id = artifacts.dataset_id
		where artifacts.format = $1
		  and artifacts.version = $2
		  and artifacts.status = $3
		  and ($4 = '' or datasets.name = $4 or artifacts.dataset_id::text = $4)
		order by artifacts.id desc
		limit 1
	`, format, version, StatusReady, dataset)

	artifact, err := scanArtifact(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Artifact{}, false, nil
		}
		return Artifact{}, false, err
	}
	return artifact, true, nil
}

func (r *PostgresRepository) LinkJob(ctx context.Context, artifactID, jobID int64) (Artifact, error) {
	row := r.pool.QueryRow(ctx, `
		update artifacts
		set created_by_job_id = $2
		where id = $1
		returning id, project_id, dataset_id, snapshot_id, artifact_type, format, version,
		          created_by_job_id, uri, manifest_uri, checksum, size, label_map_json, status, error_msg, created_at
	`, artifactID, jobID)
	return scanArtifact(row)
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, id int64, status string, errorMsg string) (Artifact, error) {
	row := r.pool.QueryRow(ctx, `
		update artifacts
		set status = $2, error_msg = $3
		where id = $1
		returning id, project_id, dataset_id, snapshot_id, artifact_type, format, version,
		          created_by_job_id, uri, manifest_uri, checksum, size, label_map_json, status, error_msg, created_at
	`, id, status, errorMsg)
	return scanArtifact(row)
}

func (r *PostgresRepository) UpdateBuildResult(ctx context.Context, id int64, result BuildResult) (Artifact, error) {
	row := r.pool.QueryRow(ctx, `
		update artifacts
		set status = $2,
		    uri = $3,
		    manifest_uri = $4,
		    checksum = $5,
		    size = $6,
		    error_msg = $7
		where id = $1
		returning id, project_id, dataset_id, snapshot_id, artifact_type, format, version,
		          created_by_job_id, uri, manifest_uri, checksum, size, label_map_json, status, error_msg, created_at
	`, id, result.Status, result.URI, result.ManifestURI, result.Checksum, result.Size, result.ErrorMsg)
	return scanArtifact(row)
}

func (r *PostgresRepository) MarkStaleBuildsFailed(ctx context.Context, reason string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		update artifacts
		set status = $1, error_msg = $2
		where status in ($3, $4, $5)
	`, StatusFailed, reason, StatusPending, StatusQueued, StatusBuilding)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func scanArtifact(row interface {
	Scan(dest ...any) error
}) (Artifact, error) {
	var artifact Artifact
	var labelMapJSON []byte
	var createdByJobID sql.NullInt64
	err := row.Scan(
		&artifact.ID,
		&artifact.ProjectID,
		&artifact.DatasetID,
		&artifact.SnapshotID,
		&artifact.ArtifactType,
		&artifact.Format,
		&artifact.Version,
		&createdByJobID,
		&artifact.URI,
		&artifact.ManifestURI,
		&artifact.Checksum,
		&artifact.Size,
		&labelMapJSON,
		&artifact.Status,
		&artifact.ErrorMsg,
		&artifact.CreatedAt,
	)
	if err != nil {
		return Artifact{}, err
	}
	if createdByJobID.Valid {
		artifact.CreatedByJobID = &createdByJobID.Int64
	}
	if len(labelMapJSON) > 0 {
		if err := json.Unmarshal(labelMapJSON, &artifact.LabelMapJSON); err != nil {
			return Artifact{}, err
		}
	}
	return artifact, nil
}

var _ Repository = (*PostgresRepository)(nil)
