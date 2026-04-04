package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func (r *PostgresRepository) CreateOrGet(in CreateJobInput) (*Job, bool, error) {
	if existing, ok, err := r.findByKey(context.Background(), in.ProjectID, in.JobType, in.IdempotencyKey); err != nil || ok {
		return existing, false, err
	}

	payloadJSON, err := json.Marshal(in.Payload)
	if err != nil {
		return nil, false, err
	}
	requiredCapabilitiesJSON, err := json.Marshal(in.RequiredCapabilities)
	if err != nil {
		return nil, false, err
	}

	row := r.pool.QueryRow(context.Background(), `
		insert into jobs (
			project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
			required_capabilities_json, idempotency_key, payload_json
		)
		values ($1, nullif($2, 0), nullif($3, 0), $4, $5, $6, $7, $8, $9)
		returning id, project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
		          required_capabilities_json, idempotency_key, worker_id, payload_json, total_items,
		          succeeded_items, failed_items, created_at, started_at, finished_at, lease_until,
		          retry_count, error_code, error_msg
	`, in.ProjectID, in.DatasetID, in.SnapshotID, in.JobType, StatusQueued, in.RequiredResourceType, requiredCapabilitiesJSON, in.IdempotencyKey, payloadJSON)

	job, err := scanJob(row)
	if err != nil {
		return nil, false, err
	}
	return job, true, nil
}

func (r *PostgresRepository) findByKey(ctx context.Context, projectID int64, jobType, key string) (*Job, bool, error) {
	row := r.pool.QueryRow(ctx, `
		select id, project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
		       required_capabilities_json, idempotency_key, worker_id, payload_json, total_items, succeeded_items, failed_items, created_at, started_at,
		       finished_at, lease_until, retry_count, error_code, error_msg
		from jobs
		where project_id = $1 and job_type = $2 and idempotency_key = $3
	`, projectID, jobType, key)

	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return job, true, nil
}

func (r *PostgresRepository) Get(id int64) (*Job, bool) {
	row := r.pool.QueryRow(context.Background(), `
		select id, project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
		       required_capabilities_json, idempotency_key, worker_id, payload_json, total_items, succeeded_items, failed_items, created_at, started_at,
		       finished_at, lease_until, retry_count, error_code, error_msg
		from jobs
		where id = $1
	`, id)

	job, err := scanJob(row)
	if err != nil {
		return nil, false
	}
	return job, true
}

func (r *PostgresRepository) ListRecentFailedJobs(projectID int64, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 5
	}

	rows, err := r.pool.Query(context.Background(), `
		select id, project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
		       required_capabilities_json, idempotency_key, worker_id, payload_json, total_items,
		       succeeded_items, failed_items, created_at, started_at, finished_at, lease_until,
		       retry_count, error_code, error_msg
		from jobs
		where project_id = $1 and status in ('failed', 'retry_waiting')
		order by coalesce(finished_at, created_at) desc
		limit $2
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *job)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateStatus(id int64, to string) error {
	job, ok := r.Get(id)
	if !ok {
		return newNotFoundError("job %d not found", id)
	}
	if err := CanTransition(job.Status, to); err != nil {
		return err
	}

	now := time.Now().UTC()
	var startedAt, finishedAt any
	if to == StatusRunning {
		startedAt = now
	}
	if to == StatusSucceeded || to == StatusSucceededWithErrors || to == StatusFailed || to == StatusCanceled {
		finishedAt = now
	}

	_, err := r.pool.Exec(context.Background(), `
		update jobs
		set status = $2,
		    started_at = coalesce($3, started_at),
		    finished_at = coalesce($4, finished_at)
		where id = $1
	`, id, to, startedAt, finishedAt)
	return err
}

func (r *PostgresRepository) Claim(id int64, workerID string, leaseUntil time.Time) (*Job, error) {
	job, ok := r.Get(id)
	if !ok {
		return nil, newNotFoundError("job %d not found", id)
	}
	if job.Status != StatusRunning {
		if err := r.UpdateStatus(id, StatusRunning); err != nil {
			return nil, err
		}
	}
	if err := r.TouchLease(id, workerID, leaseUntil); err != nil {
		return nil, err
	}
	job, ok = r.Get(id)
	if !ok {
		return nil, newNotFoundError("job %d not found", id)
	}
	return job, nil
}

func (r *PostgresRepository) TouchLease(id int64, workerID string, leaseUntil time.Time) error {
	tag, err := r.pool.Exec(context.Background(), `
		update jobs
		set worker_id = $2, lease_until = $3
		where id = $1
	`, id, workerID, leaseUntil)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return newNotFoundError("job %d not found", id)
	}
	return nil
}

func (r *PostgresRepository) UpdateProgress(id int64, workerID string, total, succeeded, failed int) error {
	job, ok := r.Get(id)
	if !ok {
		return newNotFoundError("job %d not found", id)
	}

	var startedAt any
	if job.Status != StatusRunning {
		if err := CanTransition(job.Status, StatusRunning); err != nil {
			return err
		}
		startedAt = time.Now().UTC()
	}

	_, err := r.pool.Exec(context.Background(), `
		update jobs
		set status = case when status = $2 then status else $2 end,
		    worker_id = $3,
		    total_items = $4,
		    succeeded_items = $5,
		    failed_items = $6,
		    started_at = coalesce(started_at, $7)
		where id = $1
	`, id, StatusRunning, workerID, total, succeeded, failed, startedAt)
	return err
}

func (r *PostgresRepository) Complete(id int64, workerID, status string, total, succeeded, failed int) error {
	job, ok := r.Get(id)
	if !ok {
		return newNotFoundError("job %d not found", id)
	}

	fromStatus := job.Status
	if fromStatus == StatusQueued || fromStatus == StatusRetryWaiting {
		fromStatus = StatusRunning
	}
	if err := CanTransition(fromStatus, status); err != nil {
		return err
	}

	now := time.Now().UTC()
	var startedAt any
	if job.StartedAt == nil {
		startedAt = now
	}

	_, err := r.pool.Exec(context.Background(), `
		update jobs
		set status = $2,
		    worker_id = $3,
		    total_items = $4,
		    succeeded_items = $5,
		    failed_items = $6,
		    started_at = coalesce(started_at, $7),
		    finished_at = $8
		where id = $1
	`, id, status, workerID, total, succeeded, failed, startedAt, now)
	return err
}

func (r *PostgresRepository) ListExpiredRunning(now time.Time) []*Job {
	rows, err := r.pool.Query(context.Background(), `
		select id, project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
		       required_capabilities_json, idempotency_key, worker_id, payload_json, total_items, succeeded_items, failed_items, created_at, started_at,
		       finished_at, lease_until, retry_count, error_code, error_msg
		from jobs
		where status = $1 and lease_until is not null and lease_until < $2
	`, StatusRunning, now)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []*Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil
		}
		out = append(out, job)
	}
	return out
}

func (r *PostgresRepository) IncrementRetryCount(id int64) error {
	tag, err := r.pool.Exec(context.Background(), `
		update jobs
		set retry_count = retry_count + 1
		where id = $1
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return newNotFoundError("job %d not found", id)
	}
	return nil
}

func (r *PostgresRepository) MarkRetryWaiting(id int64) error {
	return r.UpdateStatus(id, StatusRetryWaiting)
}

func (r *PostgresRepository) MarkFailed(id int64, code, msg string) error {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(context.Background(), `
		update jobs
		set status = $2, error_code = $3, error_msg = $4, finished_at = $5
		where id = $1
	`, id, StatusFailed, code, msg, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return newNotFoundError("job %d not found", id)
	}
	return nil
}

func (r *PostgresRepository) AppendEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) (Event, error) {
	if _, ok := r.Get(jobID); !ok {
		return Event{}, newNotFoundError("job %d not found", jobID)
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return Event{}, err
	}

	row := r.pool.QueryRow(context.Background(), `
		insert into job_events (job_id, item_id, event_level, event_type, message, detail_json)
		values ($1, $2, $3, $4, $5, $6)
		returning id, job_id, item_id, event_level, event_type, message, detail_json, ts
	`, jobID, itemID, level, typ, message, detailJSON)

	return scanEvent(row)
}

func (r *PostgresRepository) ListEvents(jobID int64) ([]Event, error) {
	rows, err := r.pool.Query(context.Background(), `
		select id, job_id, item_id, event_level, event_type, message, detail_json, ts
		from job_events
		where job_id = $1
		order by id asc
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) FailedRecentJobCount(projectID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(context.Background(), `
		select count(*)
		from jobs
		where project_id = $1
		  and status = $2
		  and coalesce(finished_at, created_at) >= now() - interval '24 hours'
	`, projectID, StatusFailed).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func scanJob(row interface {
	Scan(dest ...any) error
}) (*Job, error) {
	job := &Job{}
	var datasetID sql.NullInt64
	var snapshotID sql.NullInt64
	var payloadJSON []byte
	var requiredCapabilitiesJSON []byte
	var workerID *string
	var errorCode *string
	var errorMsg *string
	if err := row.Scan(
		&job.ID,
		&job.ProjectID,
		&datasetID,
		&snapshotID,
		&job.JobType,
		&job.Status,
		&job.RequiredResourceType,
		&requiredCapabilitiesJSON,
		&job.IdempotencyKey,
		&workerID,
		&payloadJSON,
		&job.TotalItems,
		&job.SucceededItems,
		&job.FailedItems,
		&job.CreatedAt,
		&job.StartedAt,
		&job.FinishedAt,
		&job.LeaseUntil,
		&job.RetryCount,
		&errorCode,
		&errorMsg,
	); err != nil {
		return nil, err
	}
	if workerID != nil {
		job.WorkerID = *workerID
	}
	if datasetID.Valid {
		job.DatasetID = datasetID.Int64
	}
	if snapshotID.Valid {
		job.SnapshotID = snapshotID.Int64
	}
	if errorCode != nil {
		job.ErrorCode = *errorCode
	}
	if errorMsg != nil {
		job.ErrorMsg = *errorMsg
	}
	if len(payloadJSON) > 0 {
		if err := json.Unmarshal(payloadJSON, &job.Payload); err != nil {
			return nil, err
		}
	}
	if len(requiredCapabilitiesJSON) > 0 {
		if err := json.Unmarshal(requiredCapabilitiesJSON, &job.RequiredCapabilities); err != nil {
			return nil, err
		}
	}
	if job.Payload == nil {
		job.Payload = map[string]any{}
	}
	return job, nil
}

func scanEvent(row interface {
	Scan(dest ...any) error
}) (Event, error) {
	var ev Event
	var detailJSON []byte
	if err := row.Scan(&ev.ID, &ev.JobID, &ev.ItemID, &ev.EventLevel, &ev.EventType, &ev.Message, &detailJSON, &ev.TS); err != nil {
		return Event{}, err
	}
	if len(detailJSON) > 0 {
		if err := json.Unmarshal(detailJSON, &ev.Detail); err != nil {
			return Event{}, err
		}
	}
	if ev.Detail == nil {
		ev.Detail = map[string]any{}
	}
	return ev, nil
}
