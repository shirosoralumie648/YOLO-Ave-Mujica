package artifacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExportQuery struct {
	pool *pgxpool.Pool
}

type ExportBundle struct {
	ProjectID   int64
	DatasetID   int64
	SnapshotID  int64
	Format      string
	Version     string
	Categories  []string
	CategoryIDs []int64
	CategoryMap map[int64]int
	Items       []ExportItem
	TotalBoxes  int
}

type ExportItem struct {
	ItemID        int64
	ObjectKey     string
	OutputName    string
	LabelFileName string
	Width         int
	Height        int
	Boxes         []YOLOBox
}

type YOLOBox struct {
	CategoryID int64
	BBoxX      float64
	BBoxY      float64
	BBoxW      float64
	BBoxH      float64
	ClassIndex int
	XCenter    float64
	YCenter    float64
	Width      float64
	Height     float64
}

func NewExportQuery(pool *pgxpool.Pool) *ExportQuery {
	return &ExportQuery{pool: pool}
}

func (q *ExportQuery) LoadSnapshotBundle(ctx context.Context, datasetID, snapshotID int64, version string) (ExportBundle, error) {
	if q == nil || q.pool == nil {
		return ExportBundle{}, errors.New("export query is not configured")
	}

	bundle, err := q.loadBundleMetadata(ctx, datasetID, snapshotID, version)
	if err != nil {
		return ExportBundle{}, err
	}
	if err := q.loadCategories(ctx, &bundle); err != nil {
		return ExportBundle{}, err
	}
	if err := q.loadItems(ctx, &bundle); err != nil {
		return ExportBundle{}, err
	}
	if err := q.loadAnnotations(ctx, &bundle); err != nil {
		return ExportBundle{}, err
	}
	return bundle, nil
}

func (q *ExportQuery) loadBundleMetadata(ctx context.Context, datasetID, snapshotID int64, version string) (ExportBundle, error) {
	var bundle ExportBundle
	var snapshotVersion string
	err := q.pool.QueryRow(ctx, `
		select d.project_id, ds.version
		from datasets d
		join dataset_snapshots ds on ds.dataset_id = d.id
		where d.id = $1 and ds.id = $2
	`, datasetID, snapshotID).Scan(&bundle.ProjectID, &snapshotVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ExportBundle{}, fmt.Errorf("dataset %d snapshot %d not found", datasetID, snapshotID)
		}
		return ExportBundle{}, err
	}

	bundle.DatasetID = datasetID
	bundle.SnapshotID = snapshotID
	bundle.Version = version
	if bundle.Version == "" {
		bundle.Version = snapshotVersion
	}
	return bundle, nil
}

func (q *ExportQuery) loadCategories(ctx context.Context, bundle *ExportBundle) error {
	rows, err := q.pool.Query(ctx, `
		select id, name
		from categories
		where project_id = $1
		order by id asc
	`, bundle.ProjectID)
	if err != nil {
		return err
	}
	defer rows.Close()

	bundle.Categories = make([]string, 0)
	bundle.CategoryIDs = make([]int64, 0)
	bundle.CategoryMap = make(map[int64]int)
	for rows.Next() {
		var categoryID int64
		var categoryName string
		if err := rows.Scan(&categoryID, &categoryName); err != nil {
			return err
		}
		bundle.CategoryMap[categoryID] = len(bundle.Categories)
		bundle.CategoryIDs = append(bundle.CategoryIDs, categoryID)
		bundle.Categories = append(bundle.Categories, categoryName)
	}
	return rows.Err()
}

func (q *ExportQuery) loadItems(ctx context.Context, bundle *ExportBundle) error {
	rows, err := q.pool.Query(ctx, `
		select id, object_key, width, height
		from dataset_items
		where dataset_id = $1
		order by id asc
	`, bundle.DatasetID)
	if err != nil {
		return err
	}
	defer rows.Close()

	usedNames := make(map[string]struct{})
	bundle.Items = make([]ExportItem, 0)
	for rows.Next() {
		var (
			item      ExportItem
			width     sql.NullInt64
			height    sql.NullInt64
		)
		if err := rows.Scan(&item.ItemID, &item.ObjectKey, &width, &height); err != nil {
			return err
		}
		if width.Valid {
			item.Width = int(width.Int64)
		}
		if height.Valid {
			item.Height = int(height.Int64)
		}
		item.OutputName = buildOutputName(item.ItemID, item.ObjectKey, usedNames)
		item.LabelFileName = buildLabelFileName(item.OutputName)
		bundle.Items = append(bundle.Items, item)
	}
	return rows.Err()
}

func (q *ExportQuery) loadAnnotations(ctx context.Context, bundle *ExportBundle) error {
	itemIndex := make(map[int64]int, len(bundle.Items))
	for i, item := range bundle.Items {
		itemIndex[item.ItemID] = i
	}

	rows, err := q.pool.Query(ctx, `
		select a.item_id, a.category_id, a.bbox_x, a.bbox_y, a.bbox_w, a.bbox_h
		from annotations a
		where a.dataset_id = $1
		  and a.created_at_snapshot_id <= $2
		  and (a.deleted_at_snapshot_id is null or a.deleted_at_snapshot_id > $2)
		order by a.item_id asc, a.category_id asc, a.id asc
	`, bundle.DatasetID, bundle.SnapshotID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			itemID     int64
			categoryID int64
			bboxX      float64
			bboxY      float64
			bboxW      float64
			bboxH      float64
		)
		if err := rows.Scan(&itemID, &categoryID, &bboxX, &bboxY, &bboxW, &bboxH); err != nil {
			return err
		}

		idx, ok := itemIndex[itemID]
		if !ok {
			return fmt.Errorf("annotation references unknown item %d", itemID)
		}
		classIndex, ok := bundle.CategoryMap[categoryID]
		if !ok {
			return fmt.Errorf("annotation references unknown category %d", categoryID)
		}

		bundle.Items[idx].Boxes = append(bundle.Items[idx].Boxes, YOLOBox{
			CategoryID: categoryID,
			BBoxX:      bboxX,
			BBoxY:      bboxY,
			BBoxW:      bboxW,
			BBoxH:      bboxH,
			ClassIndex: classIndex,
			XCenter:    bboxX + (bboxW / 2),
			YCenter:    bboxY + (bboxH / 2),
			Width:      bboxW,
			Height:     bboxH,
		})
		bundle.TotalBoxes++
	}
	return rows.Err()
}

func buildOutputName(itemID int64, objectKey string, used map[string]struct{}) string {
	base := path.Base(objectKey)
	if base == "." || base == "/" || base == "" {
		base = fmt.Sprintf("%d", itemID)
	}
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}

	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for suffix := 0; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, itemID, ext)
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d-%d%s", stem, itemID, suffix, ext)
		}
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func buildLabelFileName(outputName string) string {
	ext := path.Ext(outputName)
	stem := strings.TrimSuffix(outputName, ext)
	if stem == "" {
		stem = outputName
	}
	return stem + ".txt"
}
