package artifacts

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(a Artifact) (Artifact, error) {
	labelMapJSON, err := json.Marshal(a.LabelMapJSON)
	if err != nil {
		return Artifact{}, err
	}

	row := r.pool.QueryRow(context.Background(), `
		insert into artifacts (
			project_id, dataset_id, snapshot_id, artifact_type, format, version, uri, checksum, size,
			manifest_uri, label_map_json, status
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		returning id, project_id, dataset_id, snapshot_id, artifact_type, format, version, uri, checksum, size,
		          manifest_uri, label_map_json, status, created_at
	`, a.ProjectID, a.DatasetID, a.SnapshotID, a.ArtifactType, a.Format, a.Version, a.URI, a.Checksum, a.Size, a.ManifestURI, labelMapJSON, a.Status)

	return scanArtifact(row)
}

func (r *PostgresRepository) Get(id int64) (Artifact, bool) {
	row := r.pool.QueryRow(context.Background(), `
		select id, project_id, dataset_id, snapshot_id, artifact_type, format, version, uri, checksum, size,
		       manifest_uri, label_map_json, status, created_at
		from artifacts
		where id = $1
	`, id)

	artifact, err := scanArtifact(row)
	if err != nil {
		return Artifact{}, false
	}
	return artifact, true
}

func (r *PostgresRepository) FindByDatasetFormatVersion(dataset, format, version string) (Artifact, bool) {
	row := r.pool.QueryRow(context.Background(), `
		select artifacts.id, artifacts.project_id, artifacts.dataset_id, artifacts.snapshot_id, artifacts.artifact_type,
		       artifacts.format, artifacts.version, artifacts.uri, artifacts.checksum, artifacts.size,
		       artifacts.manifest_uri, artifacts.label_map_json, artifacts.status, artifacts.created_at
		from artifacts
		left join datasets on datasets.id = artifacts.dataset_id
		where artifacts.format = $1 and artifacts.version = $2
		  and ($3 = '' or datasets.name = $3 or artifacts.dataset_id::text = $3)
		order by artifacts.id desc
		limit 1
	`, format, version, dataset)

	artifact, err := scanArtifact(row)
	if err != nil {
		return Artifact{}, false
	}
	return artifact, true
}

func (r *PostgresRepository) UpdateReady(id int64, uri, manifestURI, checksum string, size int64) error {
	_, err := r.pool.Exec(context.Background(), `
		update artifacts
		set uri = $2,
		    manifest_uri = $3,
		    checksum = $4,
		    size = $5,
		    status = 'ready'
		where id = $1
	`, id, uri, manifestURI, checksum, size)
	return err
}

func scanArtifact(row interface {
	Scan(dest ...any) error
}) (Artifact, error) {
	var artifact Artifact
	var labelMapJSON []byte
	if err := row.Scan(
		&artifact.ID,
		&artifact.ProjectID,
		&artifact.DatasetID,
		&artifact.SnapshotID,
		&artifact.ArtifactType,
		&artifact.Format,
		&artifact.Version,
		&artifact.URI,
		&artifact.Checksum,
		&artifact.Size,
		&artifact.ManifestURI,
		&labelMapJSON,
		&artifact.Status,
		&artifact.CreatedAt,
	); err != nil {
		return Artifact{}, err
	}
	if len(labelMapJSON) > 0 {
		if err := json.Unmarshal(labelMapJSON, &artifact.LabelMapJSON); err != nil {
			return Artifact{}, err
		}
	}
	if artifact.LabelMapJSON == nil {
		artifact.LabelMapJSON = map[string]string{}
	}
	return artifact, nil
}

var _ Repository = (*PostgresRepository)(nil)
