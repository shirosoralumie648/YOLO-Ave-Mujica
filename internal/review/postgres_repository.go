package review

import (
	"context"
	"encoding/json"
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

func (r *PostgresRepository) ListPending() ([]Candidate, error) {
	rows, err := r.pool.Query(context.Background(), `
		select id, dataset_id, snapshot_id, item_id, category_id, review_status, reviewer_id, reviewed_at
		from annotation_candidates
		where review_status = 'pending'
		order by id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Candidate{}
	for rows.Next() {
		var c Candidate
		var reviewerID *string
		var reviewedAt *time.Time
		if err := rows.Scan(&c.ID, &c.DatasetID, &c.SnapshotID, &c.ItemID, &c.CategoryID, &c.ReviewStatus, &reviewerID, &reviewedAt); err != nil {
			return nil, err
		}
		if reviewerID != nil {
			c.ReviewerID = *reviewerID
		}
		if reviewedAt != nil {
			c.ReviewedAt = *reviewedAt
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) PendingCandidateCount(projectID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(context.Background(), `
		select count(*)
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		where d.project_id = $1 and c.review_status = 'pending'
	`, projectID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) Accept(candidateID int64, reviewer string) error {
	return r.transitionCandidate(candidateID, reviewer, "accepted", "review.accept", true)
}

func (r *PostgresRepository) Reject(candidateID int64, reviewer string) error {
	return r.transitionCandidate(candidateID, reviewer, "rejected", "review.reject", false)
}

func (r *PostgresRepository) ListPublishableCandidates(projectID int64) ([]PublishableCandidate, error) {
	rows, err := r.pool.Query(context.Background(), `
		select
			c.id,
			d.project_id,
			c.dataset_id,
			c.snapshot_id,
			c.item_id,
			coalesce(t.id, 0) as task_id,
			c.review_status,
			coalesce(t.priority, 'normal') as risk_level,
			coalesce(c.model_name, '') as source_model,
			coalesce(c.reviewed_at, c.created_at) as accepted_at,
			jsonb_build_object(
				'dataset_name', d.name,
				'snapshot_version', s.version,
				'task_title', coalesce(t.title, ''),
				'reviewer_id', coalesce(c.reviewer_id, '')
			) as summary
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		join dataset_snapshots s on s.id = c.snapshot_id
		left join tasks t on t.snapshot_id = c.snapshot_id and t.kind = 'review'
		where d.project_id = $1 and c.review_status = 'accepted'
		order by c.snapshot_id asc, accepted_at asc, c.id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []PublishableCandidate{}
	for rows.Next() {
		var (
			item       PublishableCandidate
			summaryRaw []byte
		)
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.DatasetID,
			&item.SnapshotID,
			&item.ItemID,
			&item.TaskID,
			&item.ReviewStatus,
			&item.RiskLevel,
			&item.SourceModel,
			&item.AcceptedAt,
			&summaryRaw,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(summaryRaw, &item.Summary); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) transitionCandidate(candidateID int64, reviewer, status, action string, promote bool) error {
	ctx := context.Background()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	type candidateRow struct {
		DatasetID  int64
		SnapshotID int64
		ItemID     int64
		CategoryID int64
		BBoxX      float64
		BBoxY      float64
		BBoxW      float64
		BBoxH      float64
		Polygon    []byte
		ModelName  *string
		IsPseudo   bool
		Status     string
	}

	var row candidateRow
	err = tx.QueryRow(ctx, `
		select dataset_id, snapshot_id, item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h, polygon_json, model_name, is_pseudo, review_status
		from annotation_candidates
		where id = $1
		for update
	`, candidateID).Scan(
		&row.DatasetID,
		&row.SnapshotID,
		&row.ItemID,
		&row.CategoryID,
		&row.BBoxX,
		&row.BBoxY,
		&row.BBoxW,
		&row.BBoxH,
		&row.Polygon,
		&row.ModelName,
		&row.IsPseudo,
		&row.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("candidate %d not found", candidateID)
		}
		return err
	}
	if row.Status != "pending" {
		return fmt.Errorf("candidate is %s", row.Status)
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		update annotation_candidates
		set review_status = $2, reviewer_id = $3, reviewed_at = $4
		where id = $1
	`, candidateID, status, reviewer, now); err != nil {
		return err
	}

	if promote {
		source := "pseudo"
		modelName := ""
		if row.ModelName != nil {
			modelName = *row.ModelName
		}
		if _, err := tx.Exec(ctx, `
			insert into annotations (
				dataset_id, item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h, polygon_json,
				source, model_name, created_at_snapshot_id, review_status, is_pseudo
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, nullif($10, ''), $11, 'verified', $12)
		`, row.DatasetID, row.ItemID, row.CategoryID, row.BBoxX, row.BBoxY, row.BBoxW, row.BBoxH, row.Polygon, source, modelName, row.SnapshotID, row.IsPseudo); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		insert into audit_logs (actor, action, resource_type, resource_id)
		values ($1, $2, 'annotation_candidate', $3)
	`, reviewer, action, fmt.Sprintf("%d", candidateID)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

var _ Repository = (*PostgresRepository)(nil)
var _ PublishableCandidateRepository = (*PostgresRepository)(nil)
