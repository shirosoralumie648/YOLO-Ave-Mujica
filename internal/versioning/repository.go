package versioning

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	ListEffectiveAnnotations(snapshotID int64) ([]Annotation, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListEffectiveAnnotations(snapshotID int64) ([]Annotation, error) {
	rows, err := r.pool.Query(context.Background(), `
		select item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h
		from annotations
		where created_at_snapshot_id <= $1
		  and (deleted_at_snapshot_id is null or deleted_at_snapshot_id > $1)
		  and review_status = 'verified'
		order by item_id asc, category_id asc, id asc
	`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Annotation{}
	for rows.Next() {
		var item Annotation
		if err := rows.Scan(&item.ItemID, &item.CategoryID, &item.BBoxX, &item.BBoxY, &item.BBoxW, &item.BBoxH); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

var _ Repository = (*PostgresRepository)(nil)
