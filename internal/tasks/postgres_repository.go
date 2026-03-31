package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateTask(ctx context.Context, in CreateTaskInput) (Task, error) {
	if err := r.validateSnapshotProject(ctx, in.ProjectID, in.SnapshotID); err != nil {
		return Task{}, err
	}

	var task Task
	var datasetID, snapshotID *int64
	var dueAt *time.Time
	err := r.pool.QueryRow(ctx, `
		insert into tasks (
			project_id, dataset_id, snapshot_id, title, description, assignee, status, priority, due_at, last_activity_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning id, project_id, dataset_id, snapshot_id, title, description, assignee, status, priority, due_at, last_activity_at, created_at, updated_at
	`, in.ProjectID, in.DatasetID, in.SnapshotID, in.Title, in.Description, in.Assignee, in.Status, in.Priority, in.DueAt, in.LastActivityAt).
		Scan(
			&task.ID,
			&task.ProjectID,
			&datasetID,
			&snapshotID,
			&task.Title,
			&task.Description,
			&task.Assignee,
			&task.Status,
			&task.Priority,
			&dueAt,
			&task.LastActivityAt,
			&task.CreatedAt,
			&task.UpdatedAt,
		)
	if err != nil {
		return Task{}, err
	}
	task.DatasetID = datasetID
	task.SnapshotID = snapshotID
	task.DueAt = dueAt
	return task, nil
}

func (r *PostgresRepository) validateSnapshotProject(ctx context.Context, projectID int64, snapshotID *int64) error {
	if snapshotID == nil {
		return nil
	}

	var matched bool
	err := r.pool.QueryRow(ctx, `
		select true
		from dataset_snapshots ds
		join datasets d on d.id = ds.dataset_id
		where ds.id = $1 and d.project_id = $2
	`, *snapshotID, projectID).Scan(&matched)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return fmt.Errorf("snapshot %d does not belong to project %d", *snapshotID, projectID)
}

func (r *PostgresRepository) ListProjectTasks(ctx context.Context, projectID int64) ([]Task, error) {
	rows, err := r.pool.Query(ctx, `
		select id, project_id, dataset_id, snapshot_id, title, description, assignee, status, priority, due_at, last_activity_at, created_at, updated_at
		from tasks
		where project_id = $1
		order by id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Task, 0)
	for rows.Next() {
		var task Task
		var datasetID, snapshotID *int64
		var dueAt *time.Time
		if err := rows.Scan(
			&task.ID,
			&task.ProjectID,
			&datasetID,
			&snapshotID,
			&task.Title,
			&task.Description,
			&task.Assignee,
			&task.Status,
			&task.Priority,
			&dueAt,
			&task.LastActivityAt,
			&task.CreatedAt,
			&task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		task.DatasetID = datasetID
		task.SnapshotID = snapshotID
		task.DueAt = dueAt
		out = append(out, task)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetTask(ctx context.Context, taskID int64) (Task, bool, error) {
	var task Task
	var datasetID, snapshotID *int64
	var dueAt *time.Time
	err := r.pool.QueryRow(ctx, `
		select id, project_id, dataset_id, snapshot_id, title, description, assignee, status, priority, due_at, last_activity_at, created_at, updated_at
		from tasks
		where id = $1
	`, taskID).Scan(
		&task.ID,
		&task.ProjectID,
		&datasetID,
		&snapshotID,
		&task.Title,
		&task.Description,
		&task.Assignee,
		&task.Status,
		&task.Priority,
		&dueAt,
		&task.LastActivityAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return Task{}, false, nil
		}
		return Task{}, false, err
	}
	task.DatasetID = datasetID
	task.SnapshotID = snapshotID
	task.DueAt = dueAt
	return task, true, nil
}

var _ Repository = (*PostgresRepository)(nil)
