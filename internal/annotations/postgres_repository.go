package annotations

import (
	"context"
	"database/sql"
	"encoding/json"
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

func (r *PostgresRepository) SaveDraft(ctx context.Context, in SaveDraftInput) (Annotation, error) {
	bodyJSON, err := marshalJSONMap(in.Body)
	if err != nil {
		return Annotation{}, err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Annotation{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	inserted, err := scanAnnotation(tx.QueryRow(ctx, `
		insert into task_annotations (
			task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
			state, revision, body_json, submitted_by, submitted_at
		)
		values ($1, $2, $3, $4, $5, $6, 1, $7, '', null)
		on conflict (task_id) do nothing
		returning id, task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
		          state, revision, body_json, submitted_by, submitted_at, created_at, updated_at
	`, in.TaskID, in.SnapshotID, in.AssetObjectKey, in.FrameIndex, in.OntologyVersion, StateDraft, bodyJSON))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Annotation{}, err
		}
		return inserted, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Annotation{}, err
	}

	current, err := scanAnnotation(tx.QueryRow(ctx, `
		select id, task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
		       state, revision, body_json, submitted_by, submitted_at, created_at, updated_at
		from task_annotations
		where task_id = $1
		for update
	`, in.TaskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Annotation{}, newNotFoundError("annotation for task %d not found", in.TaskID)
		}
		return Annotation{}, err
	}
	if current.State == StateSubmitted {
		return Annotation{}, newConflictError("annotation for task %d is already submitted", in.TaskID)
	}
	if !sameAnnotationContextPostgres(current, in) {
		return Annotation{}, newValidationError("annotation for task %d context mismatch", in.TaskID)
	}
	if in.BaseRevision > 0 && current.Revision != in.BaseRevision {
		return Annotation{}, newConflictError("task %d revision mismatch", in.TaskID)
	}

	updated, err := scanAnnotation(tx.QueryRow(ctx, `
		update task_annotations
		set state = $2,
		    revision = revision + 1,
		    body_json = $3,
		    submitted_by = '',
		    submitted_at = null,
		    updated_at = now()
		where task_id = $1
		returning id, task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
		          state, revision, body_json, submitted_by, submitted_at, created_at, updated_at
	`, in.TaskID, StateDraft, bodyJSON))
	if err != nil {
		return Annotation{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Annotation{}, err
	}
	return updated, nil
}

func (r *PostgresRepository) Submit(ctx context.Context, taskID int64, actor string) (Annotation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Annotation{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	current, err := scanAnnotation(tx.QueryRow(ctx, `
		select id, task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
		       state, revision, body_json, submitted_by, submitted_at, created_at, updated_at
		from task_annotations
		where task_id = $1
		for update
	`, taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Annotation{}, newNotFoundError("annotation for task %d not found", taskID)
		}
		return Annotation{}, err
	}

	if current.State == StateSubmitted {
		if err := tx.Commit(ctx); err != nil {
			return Annotation{}, err
		}
		return current, nil
	}

	updated, err := scanAnnotation(tx.QueryRow(ctx, `
		update task_annotations
		set state = $2,
		    revision = revision + 1,
		    submitted_by = $3,
		    submitted_at = now(),
		    updated_at = now()
		where task_id = $1
		returning id, task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
		          state, revision, body_json, submitted_by, submitted_at, created_at, updated_at
	`, taskID, StateSubmitted, actor))
	if err != nil {
		return Annotation{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Annotation{}, err
	}
	return updated, nil
}

func (r *PostgresRepository) GetByTaskID(ctx context.Context, taskID int64) (Annotation, error) {
	row := r.pool.QueryRow(ctx, `
		select id, task_id, snapshot_id, asset_object_key, frame_index, ontology_version,
		       state, revision, body_json, submitted_by, submitted_at, created_at, updated_at
		from task_annotations
		where task_id = $1
	`, taskID)
	annotation, err := scanAnnotation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Annotation{}, newNotFoundError("annotation for task %d not found", taskID)
		}
		return Annotation{}, err
	}
	return annotation, nil
}

func scanAnnotation(row interface {
	Scan(dest ...any) error
}) (Annotation, error) {
	var (
		annotation  Annotation
		bodyJSON    []byte
		frameIndex  sql.NullInt32
		submittedAt sql.NullTime
	)

	err := row.Scan(
		&annotation.ID,
		&annotation.TaskID,
		&annotation.SnapshotID,
		&annotation.AssetObjectKey,
		&frameIndex,
		&annotation.OntologyVersion,
		&annotation.State,
		&annotation.Revision,
		&bodyJSON,
		&annotation.SubmittedBy,
		&submittedAt,
		&annotation.CreatedAt,
		&annotation.UpdatedAt,
	)
	if err != nil {
		return Annotation{}, err
	}
	if frameIndex.Valid {
		frame := int(frameIndex.Int32)
		annotation.FrameIndex = &frame
	}
	if submittedAt.Valid {
		ts := submittedAt.Time
		annotation.SubmittedAt = &ts
	}
	if err := unmarshalJSONMap(bodyJSON, &annotation.Body); err != nil {
		return Annotation{}, err
	}
	return annotation, nil
}

func marshalJSONMap(in map[string]any) ([]byte, error) {
	if in == nil {
		in = map[string]any{}
	}
	return json.Marshal(in)
}

func unmarshalJSONMap(raw []byte, out *map[string]any) error {
	if len(raw) == 0 {
		*out = map[string]any{}
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode body_json: %w", err)
	}
	if *out == nil {
		*out = map[string]any{}
	}
	return nil
}

func sameAnnotationContextPostgres(existing Annotation, in SaveDraftInput) bool {
	if existing.SnapshotID != in.SnapshotID {
		return false
	}
	if existing.AssetObjectKey != in.AssetObjectKey {
		return false
	}
	if existing.OntologyVersion != in.OntologyVersion {
		return false
	}
	if existing.FrameIndex == nil && in.FrameIndex == nil {
		return true
	}
	if existing.FrameIndex == nil || in.FrameIndex == nil {
		return false
	}
	return *existing.FrameIndex == *in.FrameIndex
}

var _ Repository = (*PostgresRepository)(nil)
