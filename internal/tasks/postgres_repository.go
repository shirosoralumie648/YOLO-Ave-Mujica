package tasks

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateTask(ctx context.Context, in CreateTaskInput) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		insert into tasks (project_id, snapshot_id, title, kind, status, priority, assignee)
		values ($1, nullif($2, 0), $3, $4, $5, $6, $7)
		returning id, project_id, snapshot_id, title, kind, status, priority, assignee,
		          due_at, blocker_reason, last_activity_at, created_at, updated_at
	`, in.ProjectID, in.SnapshotID, in.Title, in.Kind, in.Status, in.Priority, in.Assignee)

	return scanTask(row)
}

func (r *PostgresRepository) ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	rows, err := r.pool.Query(ctx, `
		select id, project_id, snapshot_id, title, kind, status, priority, assignee,
		       due_at, blocker_reason, last_activity_at, created_at, updated_at
		from tasks
		where project_id = $1
		  and ($2 = '' or status = $2)
		  and ($3 = '' or assignee = $3)
		order by last_activity_at asc, id asc
	`, projectID, filter.Status, filter.Assignee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetTask(ctx context.Context, taskID int64) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		select id, project_id, snapshot_id, title, kind, status, priority, assignee,
		       due_at, blocker_reason, last_activity_at, created_at, updated_at
		from tasks
		where id = $1
	`, taskID)
	return scanTask(row)
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (Task, error) {
	var out Task
	var snapshotID sql.NullInt64
	var dueAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.ProjectID,
		&snapshotID,
		&out.Title,
		&out.Kind,
		&out.Status,
		&out.Priority,
		&out.Assignee,
		&dueAt,
		&out.BlockerReason,
		&out.LastActivityAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return Task{}, err
	}
	if snapshotID.Valid {
		value := snapshotID.Int64
		out.SnapshotID = &value
	}
	if dueAt.Valid {
		value := dueAt.Time
		out.DueAt = &value
	}
	return out, nil
}

var _ Repository = (*PostgresRepository)(nil)
