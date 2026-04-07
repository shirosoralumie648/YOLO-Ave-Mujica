package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		select c.id, d.project_id, c.job_id, c.dataset_id, c.snapshot_id, c.item_id, di.object_key, c.category_id,
		       c.bbox_x, c.bbox_y, c.bbox_w, c.bbox_h,
		       c.confidence, c.model_name, c.is_pseudo, c.review_status, c.reviewer_id, c.reviewed_at, c.created_at
		from annotation_candidates c
		join dataset_items di on di.id = c.item_id
		join datasets d on d.id = c.dataset_id
		where c.review_status in ('pending', 'queued_for_review')
		order by c.id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Candidate{}
	for rows.Next() {
		var c Candidate
		var jobID *int64
		var confidence *float64
		var reviewerID *string
		var reviewedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(
			&c.ID,
			&c.ProjectID,
			&jobID,
			&c.DatasetID,
			&c.SnapshotID,
			&c.ItemID,
			&c.ObjectKey,
			&c.CategoryID,
			&c.BBox.X,
			&c.BBox.Y,
			&c.BBox.W,
			&c.BBox.H,
			&confidence,
			&c.Source.ModelName,
			&c.Source.IsPseudo,
			&c.ReviewStatus,
			&reviewerID,
			&reviewedAt,
			&createdAt,
		); err != nil {
			return nil, err
		}
		c.Source.JobID = jobID
		c.Source.Confidence = confidence
		c.Source.CreatedAt = &createdAt
		if reviewerID != nil {
			c.ReviewerID = *reviewerID
		}
		if reviewedAt != nil {
			c.ReviewedAt = *reviewedAt
		}
		items = append(items, normalizeCandidate(c))
	}
	return items, rows.Err()
}

func (r *PostgresRepository) GetCandidate(id int64) (Candidate, bool) {
	row := r.pool.QueryRow(context.Background(), `
		select c.id, d.project_id, c.job_id, c.dataset_id, c.snapshot_id, c.item_id, di.object_key, c.category_id,
		       c.bbox_x, c.bbox_y, c.bbox_w, c.bbox_h,
		       c.confidence, c.model_name, c.is_pseudo, c.review_status, c.reviewer_id, c.reviewed_at, c.created_at
		from annotation_candidates c
		join dataset_items di on di.id = c.item_id
		join datasets d on d.id = c.dataset_id
		where c.id = $1
	`, id)

	var (
		c          Candidate
		jobID      *int64
		confidence *float64
		reviewerID *string
		reviewedAt *time.Time
		createdAt  time.Time
	)
	if err := row.Scan(
		&c.ID,
		&c.ProjectID,
		&jobID,
		&c.DatasetID,
		&c.SnapshotID,
		&c.ItemID,
		&c.ObjectKey,
		&c.CategoryID,
		&c.BBox.X,
		&c.BBox.Y,
		&c.BBox.W,
		&c.BBox.H,
		&confidence,
		&c.Source.ModelName,
		&c.Source.IsPseudo,
		&c.ReviewStatus,
		&reviewerID,
		&reviewedAt,
		&createdAt,
	); err != nil {
		return Candidate{}, false
	}
	c.Source.JobID = jobID
	c.Source.Confidence = confidence
	c.Source.CreatedAt = &createdAt
	if reviewerID != nil {
		c.ReviewerID = *reviewerID
	}
	if reviewedAt != nil {
		c.ReviewedAt = *reviewedAt
	}
	return normalizeCandidate(c), true
}

func (r *PostgresRepository) PersistCandidates(jobID int64, items []PersistCandidateInput) ([]Candidate, error) {
	ctx := context.Background()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	now := time.Now().UTC()
	out := make([]Candidate, 0, len(items))
	for _, item := range items {
		categoryID := item.CategoryID
		if categoryID <= 0 {
			err := tx.QueryRow(ctx, `
				select c.id
				from categories c
				join datasets d on d.project_id = c.project_id
				where d.id = $1 and lower(c.name) = lower($2)
			`, item.DatasetID, strings.TrimSpace(item.CategoryName)).Scan(&categoryID)
			if err != nil {
				return nil, err
			}
		}

		var candidateID int64
		err := tx.QueryRow(ctx, `
			insert into annotation_candidates (
				job_id, dataset_id, snapshot_id, item_id, category_id,
				bbox_x, bbox_y, bbox_w, bbox_h,
				confidence, model_name, is_pseudo, review_status, created_at
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, nullif($11, ''), true, 'pending', $12)
			returning id
		`, jobID, item.DatasetID, item.SnapshotID, item.ItemID, categoryID, item.BBox.X, item.BBox.Y, item.BBox.W, item.BBox.H, item.Confidence, item.ModelName, now).Scan(&candidateID)
		if err != nil {
			return nil, err
		}

		out = append(out, normalizeCandidate(Candidate{
			ID:           candidateID,
			DatasetID:    item.DatasetID,
			SnapshotID:   item.SnapshotID,
			ItemID:       item.ItemID,
			ObjectKey:    item.ObjectKey,
			CategoryID:   categoryID,
			BBox:         item.BBox,
			Status:       CandidateStatusQueuedForReview,
			ReviewStatus: CandidateStatusQueuedForReview,
			Source: CandidateSource{
				JobID:      &jobID,
				Confidence: item.Confidence,
				ModelName:  item.ModelName,
				IsPseudo:   true,
				CreatedAt:  &now,
			},
		}))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) PendingCandidateCount(projectID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(context.Background(), `
		select count(*)
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		where d.project_id = $1 and c.review_status in ('pending', 'queued_for_review')
	`, projectID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) Accept(candidateID int64, reviewer string) error {
	return r.transitionCandidate(candidateID, reviewer, "accepted", "review.accept", true, nil)
}

func (r *PostgresRepository) Reject(candidateID int64, reviewer, reasonCode string) error {
	return r.transitionCandidate(candidateID, reviewer, "rejected", "review.reject", false, map[string]any{
		"reason_code": reasonCode,
	})
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

func (r *PostgresRepository) transitionCandidate(candidateID int64, reviewer, status, action string, promote bool, detail map[string]any) error {
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
	if !isQueuedCandidateStatus(row.Status) {
		return fmt.Errorf("candidate is %s", normalizeCandidateStatus(row.Status))
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

	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		insert into audit_logs (actor, action, resource_type, resource_id, detail_json)
		values ($1, $2, 'annotation_candidate', $3, $4::jsonb)
	`, reviewer, action, fmt.Sprintf("%d", candidateID), detailJSON); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

var _ Repository = (*PostgresRepository)(nil)
