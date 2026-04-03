package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

const taskSelectColumns = `
tasks.id, tasks.project_id, tasks.snapshot_id, tasks.title, tasks.kind, tasks.status,
tasks.asset_object_key, tasks.media_kind, tasks.frame_index, tasks.ontology_version,
tasks.priority, tasks.assignee, tasks.due_at, tasks.blocker_reason,
tasks.last_activity_at, tasks.created_at, tasks.updated_at,
snapshots.version, datasets.id, datasets.name
`

const taskSelectJoins = `
left join dataset_snapshots snapshots on snapshots.id = tasks.snapshot_id
left join datasets on datasets.id = snapshots.dataset_id
`

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateTask(ctx context.Context, in CreateTaskInput) (Task, error) {
	lastActivityAt := in.LastActivityAt
	if lastActivityAt.IsZero() {
		lastActivityAt = time.Now().UTC()
	}

	if in.SnapshotID != nil {
		var snapshotProjectID int64
		err := r.pool.QueryRow(ctx, `
			select datasets.project_id
			from dataset_snapshots
			join datasets on datasets.id = dataset_snapshots.dataset_id
			where dataset_snapshots.id = $1
		`, *in.SnapshotID).Scan(&snapshotProjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Task{}, fmt.Errorf("snapshot_id %d not found", *in.SnapshotID)
			}
			return Task{}, err
		}
		if snapshotProjectID != in.ProjectID {
			return Task{}, fmt.Errorf("snapshot_id %d belongs to project %d, not %d", *in.SnapshotID, snapshotProjectID, in.ProjectID)
		}
	}

	row := r.pool.QueryRow(ctx, `
		with inserted as (
			insert into tasks (
				project_id, snapshot_id, title, kind, status, priority, assignee,
				due_at, blocker_reason, last_activity_at, asset_object_key, media_kind,
				frame_index, ontology_version
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			returning id, project_id, snapshot_id, title, kind, status, priority,
			          asset_object_key, media_kind, frame_index, ontology_version,
			          assignee, due_at, blocker_reason, last_activity_at, created_at, updated_at
		)
		select `+taskSelectColumns+`
		from inserted tasks
		`+taskSelectJoins+`
	`, in.ProjectID, in.SnapshotID, in.Title, in.Kind, in.Status, in.Priority, in.Assignee, in.DueAt, in.BlockerReason, lastActivityAt, in.AssetObjectKey, in.MediaKind, in.FrameIndex, in.OntologyVersion)

	return scanTask(row)
}

func (r *PostgresRepository) ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	rows, err := r.pool.Query(ctx, `
		select `+taskSelectColumns+`
		from tasks
		`+taskSelectJoins+`
		where tasks.project_id = $1
		  and ($2 = '' or tasks.status = $2)
		  and ($3 = '' or tasks.kind = $3)
		  and ($4 = '' or tasks.assignee = $4)
		  and ($5 = '' or tasks.priority = $5)
		  and ($6::bigint is null or tasks.snapshot_id = $6)
		order by tasks.id asc
	`, projectID, filter.Status, filter.Kind, filter.Assignee, filter.Priority, filter.SnapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *PostgresRepository) GetTask(ctx context.Context, taskID int64) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		select `+taskSelectColumns+`
		from tasks
		`+taskSelectJoins+`
		where tasks.id = $1
	`, taskID)

	return scanTask(row)
}

func (r *PostgresRepository) TransitionTask(ctx context.Context, taskID int64, in TransitionTaskInput) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		with updated as (
			update tasks
			set status = $2,
			    blocker_reason = $3,
			    last_activity_at = $4,
			    updated_at = now()
			where id = $1
			returning id, project_id, snapshot_id, title, kind, status, priority,
			          asset_object_key, media_kind, frame_index, ontology_version,
			          assignee, due_at, blocker_reason, last_activity_at, created_at, updated_at
		)
		select `+taskSelectColumns+`
		from updated tasks
		`+taskSelectJoins+`
	`, taskID, in.Status, in.BlockerReason, in.LastActivityAt)

	return scanTask(row)
}

func scanTask(row interface {
	Scan(dest ...any) error
}) (Task, error) {
	var task Task
	var snapshotID sql.NullInt64
	var snapshotVersion sql.NullString
	var datasetID sql.NullInt64
	var datasetName sql.NullString
	err := row.Scan(
		&task.ID,
		&task.ProjectID,
		&snapshotID,
		&task.Title,
		&task.Kind,
		&task.Status,
		&task.AssetObjectKey,
		&task.MediaKind,
		&task.FrameIndex,
		&task.OntologyVersion,
		&task.Priority,
		&task.Assignee,
		&task.DueAt,
		&task.BlockerReason,
		&task.LastActivityAt,
		&task.CreatedAt,
		&task.UpdatedAt,
		&snapshotVersion,
		&datasetID,
		&datasetName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, err
		}
		return Task{}, err
	}
	if snapshotID.Valid {
		task.SnapshotID = &snapshotID.Int64
	}
	if snapshotVersion.Valid {
		task.SnapshotVersion = snapshotVersion.String
	}
	if datasetID.Valid {
		task.DatasetID = datasetID.Int64
	}
	if datasetName.Valid {
		task.DatasetName = datasetName.String
	}
	return task, nil
}

var _ Repository = (*PostgresRepository)(nil)
