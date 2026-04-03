package publish

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Batch{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	ruleSummary, err := marshalJSONMap(in.RuleSummary)
	if err != nil {
		return Batch{}, err
	}

	var batchID int64
	if err := tx.QueryRow(ctx, `
		insert into publish_batches (project_id, snapshot_id, source, status, rule_summary_json)
		values ($1, $2, $3, $4, $5)
		returning id
	`, in.ProjectID, in.SnapshotID, in.Source, StatusDraft, ruleSummary).Scan(&batchID); err != nil {
		return Batch{}, err
	}

	if err := insertBatchItems(ctx, tx, batchID, in.SnapshotID, in.Items); err != nil {
		return Batch{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Batch{}, err
	}

	return r.GetBatch(ctx, batchID)
}

func (r *PostgresRepository) GetBatch(ctx context.Context, batchID int64) (Batch, error) {
	var (
		batch           Batch
		ruleSummaryJSON []byte
		reviewApproved  sql.NullTime
		ownerDecided    sql.NullTime
	)

	err := r.pool.QueryRow(ctx, `
		select id, project_id, snapshot_id, source, status, rule_summary_json, owner_edit_version,
		       review_approved_at, review_approved_by, owner_decided_at, owner_decided_by,
		       created_at, updated_at
		from publish_batches
		where id = $1
	`, batchID).Scan(
		&batch.ID,
		&batch.ProjectID,
		&batch.SnapshotID,
		&batch.Source,
		&batch.Status,
		&ruleSummaryJSON,
		&batch.OwnerEditVersion,
		&reviewApproved,
		&batch.ReviewApprovedBy,
		&ownerDecided,
		&batch.OwnerDecidedBy,
		&batch.CreatedAt,
		&batch.UpdatedAt,
	)
	if err != nil {
		return Batch{}, err
	}
	if err := unmarshalJSONMap(ruleSummaryJSON, &batch.RuleSummary); err != nil {
		return Batch{}, err
	}
	if reviewApproved.Valid {
		batch.ReviewApprovedAt = &reviewApproved.Time
	}
	if ownerDecided.Valid {
		batch.OwnerDecidedAt = &ownerDecided.Time
	}

	items, err := r.listBatchItems(ctx, batchID)
	if err != nil {
		return Batch{}, err
	}
	batch.Items = items

	feedback, err := r.listFeedback(ctx, batchID)
	if err != nil {
		return Batch{}, err
	}
	batch.Feedback = feedback

	return batch, nil
}

func (r *PostgresRepository) GetRecord(ctx context.Context, recordID int64) (Record, error) {
	var (
		record      Record
		summaryJSON []byte
	)

	err := r.pool.QueryRow(ctx, `
		select id, project_id, snapshot_id, publish_batch_id, status, summary_json,
		       approved_by_owner, approved_at, created_at
		from publish_records
		where id = $1
	`, recordID).Scan(
		&record.ID,
		&record.ProjectID,
		&record.SnapshotID,
		&record.PublishBatchID,
		&record.Status,
		&summaryJSON,
		&record.ApprovedByOwner,
		&record.ApprovedAt,
		&record.CreatedAt,
	)
	if err != nil {
		return Record{}, err
	}
	if err := unmarshalJSONMap(summaryJSON, &record.Summary); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (r *PostgresRepository) ReplaceBatchItems(ctx context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Batch{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		delete from publish_batch_items
		where publish_batch_id = $1
	`, batchID); err != nil {
		return Batch{}, err
	}

	var batchSnapshotID int64
	if err := tx.QueryRow(ctx, `
		select snapshot_id
		from publish_batches
		where id = $1
		for update
	`, batchID).Scan(&batchSnapshotID); err != nil {
		return Batch{}, err
	}

	if err := insertBatchItems(ctx, tx, batchID, batchSnapshotID, in.Items); err != nil {
		return Batch{}, err
	}

	tag, err := tx.Exec(ctx, `
		update publish_batches
		set status = $2,
		    owner_edit_version = owner_edit_version + 1,
		    review_approved_at = null,
		    review_approved_by = '',
		    owner_decided_at = null,
		    owner_decided_by = '',
		    updated_at = now()
		where id = $1
	`, batchID, StatusOwnerChangesRequested)
	if err != nil {
		return Batch{}, err
	}
	if tag.RowsAffected() == 0 {
		return Batch{}, pgx.ErrNoRows
	}

	if err := tx.Commit(ctx); err != nil {
		return Batch{}, err
	}

	return r.GetBatch(ctx, batchID)
}

func (r *PostgresRepository) ApplyReviewDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, item := range feedback {
		if _, err := insertFeedback(ctx, tx, batchID, nil, item); err != nil {
			return err
		}
	}

	status := StatusOwnerChangesRequested
	reviewApprovedBy := ""
	var reviewApprovedAt any

	switch decision {
	case ReviewDecisionApprove:
		status = StatusReviewApproved
		reviewApprovedBy = actor
		reviewApprovedAt = time.Now().UTC()
	case ReviewDecisionReject:
		status = StatusRejected
		reviewApprovedAt = nil
	case ReviewDecisionRework:
		status = StatusOwnerChangesRequested
		reviewApprovedAt = nil
	default:
		return fmt.Errorf("unsupported review decision %q", decision)
	}

	tag, err := tx.Exec(ctx, `
		update publish_batches
		set status = $2,
		    review_approved_at = $3,
		    review_approved_by = $4,
		    updated_at = now()
		where id = $1
	`, batchID, status, reviewApprovedAt, reviewApprovedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) ApplyOwnerDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) (Record, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Record{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, item := range feedback {
		if _, err := insertFeedback(ctx, tx, batchID, nil, item); err != nil {
			return Record{}, err
		}
	}

	var (
		projectID  int64
		snapshotID int64
	)
	if err := tx.QueryRow(ctx, `
		select project_id, snapshot_id
		from publish_batches
		where id = $1
		for update
	`, batchID).Scan(&projectID, &snapshotID); err != nil {
		return Record{}, err
	}

	now := time.Now().UTC()

	switch decision {
	case OwnerDecisionApprove:
		summary, err := marshalJSONMap(map[string]any{
			"decision":    OwnerDecisionApprove,
			"batch_id":    batchID,
			"snapshot_id": snapshotID,
		})
		if err != nil {
			return Record{}, err
		}

		var (
			record     Record
			summaryRaw []byte
		)
		if err := tx.QueryRow(ctx, `
			insert into publish_records (
				project_id, snapshot_id, publish_batch_id, status, summary_json, approved_by_owner, approved_at
			)
			values ($1, $2, $3, $4, $5, $6, $7)
			returning id, project_id, snapshot_id, publish_batch_id, status, summary_json,
			          approved_by_owner, approved_at, created_at
		`, projectID, snapshotID, batchID, StatusPublished, summary, actor, now).Scan(
			&record.ID,
			&record.ProjectID,
			&record.SnapshotID,
			&record.PublishBatchID,
			&record.Status,
			&summaryRaw,
			&record.ApprovedByOwner,
			&record.ApprovedAt,
			&record.CreatedAt,
		); err != nil {
			return Record{}, err
		}
		if err := unmarshalJSONMap(summaryRaw, &record.Summary); err != nil {
			return Record{}, err
		}

		if _, err := tx.Exec(ctx, `
			update publish_batches
			set status = $2,
			    owner_decided_at = $3,
			    owner_decided_by = $4,
			    updated_at = now()
			where id = $1
		`, batchID, StatusPublished, now, actor); err != nil {
			return Record{}, err
		}

		if err := tx.Commit(ctx); err != nil {
			return Record{}, err
		}
		return record, nil
	case OwnerDecisionReject:
		if _, err := tx.Exec(ctx, `
			update publish_batches
			set status = $2,
			    owner_decided_at = $3,
			    owner_decided_by = $4,
			    updated_at = now()
			where id = $1
		`, batchID, StatusRejected, now, actor); err != nil {
			return Record{}, err
		}
	case OwnerDecisionRework:
		if _, err := tx.Exec(ctx, `
			update publish_batches
			set status = $2,
			    owner_decided_at = $3,
			    owner_decided_by = $4,
			    updated_at = now()
			where id = $1
		`, batchID, StatusOwnerChangesRequested, now, actor); err != nil {
			return Record{}, err
		}
	default:
		return Record{}, fmt.Errorf("unsupported owner decision %q", decision)
	}

	if err := tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return Record{}, nil
}

func (r *PostgresRepository) AddBatchFeedback(ctx context.Context, batchID int64, in CreateFeedbackInput) (Feedback, error) {
	return insertFeedback(ctx, r.pool, batchID, nil, in)
}

func (r *PostgresRepository) AddItemFeedback(ctx context.Context, batchID, itemID int64, in CreateFeedbackInput) (Feedback, error) {
	return insertFeedback(ctx, r.pool, batchID, &itemID, in)
}

func (r *PostgresRepository) ListSuggestedCandidates(ctx context.Context, projectID int64) ([]SuggestedCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		select
			c.id,
			coalesce(t.id, 0) as task_id,
			c.dataset_id,
			c.snapshot_id,
			coalesce(t.priority, 'normal') as risk_level,
			coalesce(c.model_name, '') as source_model,
			date_trunc('hour', coalesce(c.reviewed_at, c.created_at)) as accepted_window,
			jsonb_build_object(
				'task', jsonb_build_object('id', coalesce(t.id, 0), 'title', coalesce(t.title, '')),
				'snapshot', jsonb_build_object('id', s.id, 'version', s.version)
			) as item_payload
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		join dataset_snapshots s on s.id = c.snapshot_id
		left join tasks t on t.snapshot_id = c.snapshot_id and t.kind = 'review'
		where d.project_id = $1 and c.review_status = 'accepted'
		order by c.snapshot_id asc, accepted_window asc, c.id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string]*SuggestedCandidate)
	order := make([]string, 0)

	for rows.Next() {
		var (
			candidateID    int64
			taskID         int64
			datasetID      int64
			snapshotID     int64
			riskLevel      string
			sourceModel    string
			acceptedWindow time.Time
			itemPayloadRaw []byte
			itemPayload    map[string]any
		)

		if err := rows.Scan(&candidateID, &taskID, &datasetID, &snapshotID, &riskLevel, &sourceModel, &acceptedWindow, &itemPayloadRaw); err != nil {
			return nil, err
		}
		if err := unmarshalJSONMap(itemPayloadRaw, &itemPayload); err != nil {
			return nil, err
		}

		key := fmt.Sprintf("%d:%s:%s:%s", snapshotID, riskLevel, sourceModel, acceptedWindow.Format(time.RFC3339))
		group := grouped[key]
		if group == nil {
			group = &SuggestedCandidate{
				SnapshotID:    snapshotID,
				SuggestionKey: key,
				Summary: map[string]any{
					"risk_level":      riskLevel,
					"source_model":    sourceModel,
					"accepted_window": acceptedWindow.Format(time.RFC3339),
				},
			}
			grouped[key] = group
			order = append(order, key)
		}

		group.Items = append(group.Items, SuggestedCandidateItem{
			CandidateID: candidateID,
			TaskID:      taskID,
			DatasetID:   datasetID,
			ItemPayload: itemPayload,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SuggestedCandidate, 0, len(order))
	for _, key := range order {
		out = append(out, *grouped[key])
	}
	return out, nil
}

func (r *PostgresRepository) listBatchItems(ctx context.Context, batchID int64) ([]BatchItem, error) {
	rows, err := r.pool.Query(ctx, `
		select id, publish_batch_id, candidate_id, task_id, dataset_id, snapshot_id,
		       item_payload_json, position, created_at, updated_at
		from publish_batch_items
		where publish_batch_id = $1
		order by position asc, id asc
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []BatchItem{}
	for rows.Next() {
		var (
			item           BatchItem
			itemPayloadRaw []byte
		)
		if err := rows.Scan(
			&item.ID,
			&item.PublishBatchID,
			&item.CandidateID,
			&item.TaskID,
			&item.DatasetID,
			&item.SnapshotID,
			&itemPayloadRaw,
			&item.Position,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalJSONMap(itemPayloadRaw, &item.ItemPayload); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) listFeedback(ctx context.Context, batchID int64) ([]Feedback, error) {
	rows, err := r.pool.Query(ctx, `
		select id, publish_batch_id, publish_batch_item_id, scope, stage, action,
		       reason_code, severity, influence_weight, comment, created_by, created_at
		from publish_feedback
		where publish_batch_id = $1
		order by created_at asc, id asc
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feedback := []Feedback{}
	for rows.Next() {
		var (
			item               Feedback
			publishBatchItemID sql.NullInt64
		)
		if err := rows.Scan(
			&item.ID,
			&item.PublishBatchID,
			&publishBatchItemID,
			&item.Scope,
			&item.Stage,
			&item.Action,
			&item.ReasonCode,
			&item.Severity,
			&item.InfluenceWeight,
			&item.Comment,
			&item.CreatedBy,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if publishBatchItemID.Valid {
			item.PublishBatchItemID = publishBatchItemID.Int64
		}
		feedback = append(feedback, item)
	}
	return feedback, rows.Err()
}

func insertBatchItems(ctx context.Context, db interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, batchID, batchSnapshotID int64, items []CreateBatchItemInput) error {
	for i, item := range items {
		if item.SnapshotID != batchSnapshotID {
			return fmt.Errorf("batch item snapshot_id %d does not match batch snapshot_id %d", item.SnapshotID, batchSnapshotID)
		}
		itemPayload, err := marshalJSONMap(item.ItemPayload)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			insert into publish_batch_items (
				publish_batch_id, candidate_id, task_id, snapshot_id, dataset_id, item_payload_json, position
			) values ($1, $2, $3, $4, $5, $6, $7)
		`, batchID, item.CandidateID, item.TaskID, item.SnapshotID, item.DatasetID, itemPayload, i); err != nil {
			return err
		}
	}
	return nil
}

func insertFeedback(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, batchID int64, itemID *int64, in CreateFeedbackInput) (Feedback, error) {
	scope := in.Scope
	if scope == "" {
		scope = FeedbackScopeBatch
		if itemID != nil {
			scope = FeedbackScopeItem
		}
	}

	stage := in.Stage
	if stage == "" {
		stage = FeedbackStageReview
	}

	action := in.Action
	if action == "" {
		action = FeedbackActionComment
	}

	severity := in.Severity
	if severity == "" {
		severity = "medium"
	}

	reasonCode := in.ReasonCode
	if reasonCode == "" {
		reasonCode = "unspecified"
	}

	weight := in.InfluenceWeight
	if weight == 0 {
		weight = 1
	}

	var (
		feedback           Feedback
		publishBatchItemID sql.NullInt64
	)
	if itemID != nil {
		publishBatchItemID = sql.NullInt64{Int64: *itemID, Valid: true}
	}

	if err := db.QueryRow(ctx, `
		insert into publish_feedback (
			publish_batch_id, publish_batch_item_id, scope, stage, action,
			reason_code, severity, influence_weight, comment, created_by
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning id, publish_batch_id, publish_batch_item_id, scope, stage, action,
		          reason_code, severity, influence_weight, comment, created_by, created_at
	`, batchID, publishBatchItemID, scope, stage, action, reasonCode, severity, weight, in.Comment, in.Actor).Scan(
		&feedback.ID,
		&feedback.PublishBatchID,
		&publishBatchItemID,
		&feedback.Scope,
		&feedback.Stage,
		&feedback.Action,
		&feedback.ReasonCode,
		&feedback.Severity,
		&feedback.InfluenceWeight,
		&feedback.Comment,
		&feedback.CreatedBy,
		&feedback.CreatedAt,
	); err != nil {
		return Feedback{}, err
	}
	if publishBatchItemID.Valid {
		feedback.PublishBatchItemID = publishBatchItemID.Int64
	}

	return feedback, nil
}

func marshalJSONMap(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func unmarshalJSONMap(raw []byte, target *map[string]any) error {
	if len(raw) == 0 {
		*target = map[string]any{}
		return nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	*target = decoded
	return nil
}

var _ Repository = (*PostgresRepository)(nil)
