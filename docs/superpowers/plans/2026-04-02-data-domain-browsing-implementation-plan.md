# Data Domain Browsing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the read-only Data navigation slice so users can browse datasets, inspect one dataset and one snapshot, compare snapshots with the existing diff API, and jump from task detail back into the data domain.

**Architecture:** Extend `internal/datahub` with focused browse query models and read endpoints instead of pushing aggregation into the React client. Keep snapshot diff on the existing `internal/versioning` endpoint, then add a new `apps/web/src/features/data` bundle that follows the current App Shell, React Router, and React Query patterns while keeping publish semantics explicitly out of scope.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, React 19, TypeScript, React Router, TanStack Query, Vitest, Testing Library.

---

## File Structure

**Create**

- `internal/datahub/browse_models.go`
- `internal/datahub/errors.go`
- `apps/web/src/features/data/api.ts`
- `apps/web/src/features/data/dataset-list-page.tsx`
- `apps/web/src/features/data/dataset-detail-page.tsx`
- `apps/web/src/features/data/snapshot-detail-page.tsx`
- `apps/web/src/features/data/snapshot-diff-page.tsx`
- `apps/web/src/features/data/data-pages.test.tsx`

**Modify**

- `internal/datahub/repository.go`
- `internal/datahub/postgres_repository.go`
- `internal/datahub/postgres_repository_test.go`
- `internal/datahub/service.go`
- `internal/datahub/service_test.go`
- `internal/datahub/handler.go`
- `internal/datahub/handler_test.go`
- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `apps/web/src/app/layout/app-shell.tsx`
- `apps/web/src/app/router.tsx`
- `apps/web/src/app/styles.css`
- `apps/web/src/features/tasks/task-detail-page.tsx`
- `apps/web/src/features/tasks/task-detail-page.test.tsx`

## Task 1: Add Data Browse Query Models And Repository Reads

**Files:**

- Create: `internal/datahub/browse_models.go`
- Create: `internal/datahub/errors.go`
- Modify: `internal/datahub/repository.go`
- Modify: `internal/datahub/postgres_repository.go`
- Modify: `internal/datahub/postgres_repository_test.go`
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/service_test.go`

- [ ] **Step 1: Write the failing repository and service tests**

Append these tests to `internal/datahub/service_test.go`:

```go
func TestServiceListDatasetsIncludesCountsAndLatestSnapshot(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "dock-night",
		Bucket:    "platform-dev",
		Prefix:    "train/night",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 2,
		Name:      "other-project",
		Bucket:    "platform-dev",
		Prefix:    "shadow",
	}); err != nil {
		t.Fatalf("create secondary dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/night/a.jpg", "train/night/b.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "baseline"}); err != nil {
		t.Fatalf("create baseline snapshot: %v", err)
	}
	latest, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "relabel batch"})
	if err != nil {
		t.Fatalf("create second snapshot: %v", err)
	}

	summaries, err := svc.ListDatasets(1)
	if err != nil {
		t.Fatalf("list datasets: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 dataset for project 1, got %d", len(summaries))
	}

	summary := summaries[0]
	if summary.ID != dataset.ID || summary.Name != "dock-night" {
		t.Fatalf("unexpected dataset summary identity: %+v", summary)
	}
	if summary.ItemCount != 2 {
		t.Fatalf("expected 2 items, got %d", summary.ItemCount)
	}
	if summary.SnapshotCount != 2 {
		t.Fatalf("expected 2 snapshots, got %d", summary.SnapshotCount)
	}
	if summary.LatestSnapshotID == nil || *summary.LatestSnapshotID != latest.ID {
		t.Fatalf("expected latest snapshot id %d, got %+v", latest.ID, summary.LatestSnapshotID)
	}
	if summary.LatestSnapshotVersion != latest.Version {
		t.Fatalf("expected latest snapshot version %q, got %q", latest.Version, summary.LatestSnapshotVersion)
	}
}

func TestServiceGetDatasetDetailAndSnapshotDetail(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "yard-day",
		Bucket:    "platform-dev",
		Prefix:    "train/day",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/day/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	parent, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "seed"})
	if err != nil {
		t.Fatalf("create parent snapshot: %v", err)
	}
	child, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{
		BasedOnSnapshotID: &parent.ID,
		Note:              "qa relabel",
	})
	if err != nil {
		t.Fatalf("create child snapshot: %v", err)
	}
	if _, err := svc.ImportSnapshot(child.ID, ImportSnapshotInput{
		Format: "yolo",
		Entries: []ImportedAnnotation{
			{
				ObjectKey:    "train/day/a.jpg",
				CategoryName: "person",
				BBoxX:        0.1,
				BBoxY:        0.2,
				BBoxW:        0.3,
				BBoxH:        0.4,
			},
		},
	}); err != nil {
		t.Fatalf("import snapshot annotations: %v", err)
	}

	detail, err := svc.GetDatasetDetail(dataset.ID)
	if err != nil {
		t.Fatalf("get dataset detail: %v", err)
	}
	if detail.ItemCount != 1 || detail.SnapshotCount != 2 {
		t.Fatalf("unexpected dataset detail counts: %+v", detail)
	}
	if detail.LatestSnapshotID == nil || *detail.LatestSnapshotID != child.ID {
		t.Fatalf("expected latest snapshot id %d, got %+v", child.ID, detail.LatestSnapshotID)
	}

	snapshotDetail, err := svc.GetSnapshotDetail(child.ID)
	if err != nil {
		t.Fatalf("get snapshot detail: %v", err)
	}
	if snapshotDetail.DatasetID != dataset.ID || snapshotDetail.DatasetName != dataset.Name {
		t.Fatalf("unexpected snapshot detail identity: %+v", snapshotDetail)
	}
	if snapshotDetail.BasedOnSnapshotID == nil || *snapshotDetail.BasedOnSnapshotID != parent.ID {
		t.Fatalf("expected parent snapshot id %d, got %+v", parent.ID, snapshotDetail.BasedOnSnapshotID)
	}
	if snapshotDetail.AnnotationCount != 1 {
		t.Fatalf("expected 1 annotation, got %d", snapshotDetail.AnnotationCount)
	}
}
```

Append this integration test and helper to `internal/datahub/postgres_repository_test.go`:

```go
import (
	"context"
	"os"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)
```

```go
func findDatasetSummary(items []DatasetSummary, datasetID int64) (DatasetSummary, bool) {
	for _, item := range items {
		if item.ID == datasetID {
			return item, true
		}
	}
	return DatasetSummary{}, false
}

func TestPostgresRepositoryListDatasetsAndSnapshotDetail(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	projectName := "datahub-browse-project-" + time.Now().UTC().Format("20060102150405.000000000")
	var projectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, 'test-owner')
		returning id
	`, projectName).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	repo := NewPostgresRepository(pool)
	dataset, err := repo.CreateDataset(ctx, CreateDatasetInput{
		ProjectID: projectID,
		Name:      "integration-browse",
		Bucket:    "platform-dev",
		Prefix:    "train/integration",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := repo.InsertItems(ctx, dataset.ID, []string{"train/integration/a.jpg", "train/integration/b.jpg"}); err != nil {
		t.Fatalf("insert items: %v", err)
	}
	parent, err := repo.CreateSnapshot(ctx, dataset.ID, CreateSnapshotInput{Note: "baseline"})
	if err != nil {
		t.Fatalf("create parent snapshot: %v", err)
	}
	child, err := repo.CreateSnapshot(ctx, dataset.ID, CreateSnapshotInput{
		BasedOnSnapshotID: &parent.ID,
		Note:              "candidate",
	})
	if err != nil {
		t.Fatalf("create child snapshot: %v", err)
	}

	item, err := repo.GetItemByObjectKey(ctx, dataset.ID, "train/integration/a.jpg")
	if err != nil {
		t.Fatalf("get item by object key: %v", err)
	}
	categoryID, err := repo.EnsureCategory(ctx, projectID, "person")
	if err != nil {
		t.Fatalf("ensure category: %v", err)
	}
	if err := repo.CreateAnnotation(ctx, child.ID, dataset.ID, item.ID, item.ObjectKey, categoryID, "person", 0.1, 0.2, 0.3, 0.4); err != nil {
		t.Fatalf("create annotation: %v", err)
	}

	summaries, err := repo.ListDatasets(ctx, projectID)
	if err != nil {
		t.Fatalf("list datasets: %v", err)
	}
	summary, ok := findDatasetSummary(summaries, dataset.ID)
	if !ok {
		t.Fatalf("expected dataset %d in summaries: %+v", dataset.ID, summaries)
	}
	if summary.ItemCount != 2 || summary.SnapshotCount != 2 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.LatestSnapshotID == nil || *summary.LatestSnapshotID != child.ID {
		t.Fatalf("expected latest snapshot id %d, got %+v", child.ID, summary.LatestSnapshotID)
	}

	detail, err := repo.GetDatasetDetail(ctx, dataset.ID)
	if err != nil {
		t.Fatalf("get dataset detail: %v", err)
	}
	if detail.ItemCount != 2 || detail.SnapshotCount != 2 {
		t.Fatalf("unexpected dataset detail: %+v", detail)
	}

	snapshotDetail, err := repo.GetSnapshotDetail(ctx, child.ID)
	if err != nil {
		t.Fatalf("get snapshot detail: %v", err)
	}
	if snapshotDetail.DatasetName != dataset.Name {
		t.Fatalf("expected dataset name %q, got %q", dataset.Name, snapshotDetail.DatasetName)
	}
	if snapshotDetail.AnnotationCount != 1 {
		t.Fatalf("expected 1 annotation, got %d", snapshotDetail.AnnotationCount)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub -count=1 -v
```

Expected: FAIL with undefined browse types or methods such as `DatasetSummary`, `ListDatasets`, `GetDatasetDetail`, or `GetSnapshotDetail`.

- [ ] **Step 3: Add the browse models, not-found errors, and repository/service implementations**

Create `internal/datahub/browse_models.go`:

```go
package datahub

type DatasetSummary struct {
	ID                    int64  `json:"id"`
	ProjectID             int64  `json:"project_id"`
	Name                  string `json:"name"`
	Bucket                string `json:"bucket"`
	Prefix                string `json:"prefix"`
	ItemCount             int    `json:"item_count"`
	LatestSnapshotID      *int64 `json:"latest_snapshot_id,omitempty"`
	LatestSnapshotVersion string `json:"latest_snapshot_version,omitempty"`
	SnapshotCount         int    `json:"snapshot_count"`
}

type DatasetDetail struct {
	ID                    int64  `json:"id"`
	ProjectID             int64  `json:"project_id"`
	Name                  string `json:"name"`
	Bucket                string `json:"bucket"`
	Prefix                string `json:"prefix"`
	ItemCount             int    `json:"item_count"`
	SnapshotCount         int    `json:"snapshot_count"`
	LatestSnapshotID      *int64 `json:"latest_snapshot_id,omitempty"`
	LatestSnapshotVersion string `json:"latest_snapshot_version,omitempty"`
}

type SnapshotDetail struct {
	ID                int64  `json:"id"`
	DatasetID         int64  `json:"dataset_id"`
	DatasetName       string `json:"dataset_name"`
	ProjectID         int64  `json:"project_id"`
	Version           string `json:"version"`
	BasedOnSnapshotID *int64 `json:"based_on_snapshot_id,omitempty"`
	Note              string `json:"note"`
	AnnotationCount   int    `json:"annotation_count"`
}
```

Create `internal/datahub/errors.go`:

```go
package datahub

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("datahub: not found")

func wrapNotFound(resource string, id int64) error {
	return fmt.Errorf("%w: %s %d not found", ErrNotFound, resource, id)
}
```

Update `internal/datahub/repository.go`:

```go
import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)
```

```go
type Repository interface {
	CreateDataset(ctx context.Context, in CreateDatasetInput) (Dataset, error)
	GetDataset(ctx context.Context, datasetID int64) (Dataset, error)
	ListDatasets(ctx context.Context, projectID int64) ([]DatasetSummary, error)
	GetDatasetDetail(ctx context.Context, datasetID int64) (DatasetDetail, error)
	CreateSnapshot(ctx context.Context, datasetID int64, in CreateSnapshotInput) (Snapshot, error)
	GetSnapshot(ctx context.Context, snapshotID int64) (Snapshot, error)
	GetSnapshotDetail(ctx context.Context, snapshotID int64) (SnapshotDetail, error)
	ListSnapshots(ctx context.Context, datasetID int64) ([]Snapshot, error)
	InsertItems(ctx context.Context, datasetID int64, objectKeys []string) (int, error)
	UpsertScannedItems(ctx context.Context, datasetID int64, objects []ScannedObject) (int, error)
	ListItems(ctx context.Context, datasetID int64) ([]DatasetItem, error)
	GetItemByObjectKey(ctx context.Context, datasetID int64, objectKey string) (DatasetItem, error)
	EnsureCategory(ctx context.Context, projectID int64, categoryName string) (int64, error)
	CreateAnnotation(ctx context.Context, snapshotID, datasetID, itemID int64, objectKey string, categoryID int64, categoryName string, bboxX, bboxY, bboxW, bboxH float64) error
}
```

Add the browse helpers and methods to the in-memory implementation in `internal/datahub/repository.go`:

```go
func (r *InMemoryRepository) ListDatasets(_ context.Context, projectID int64) ([]DatasetSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]int64, 0, len(r.datasets))
	for id, dataset := range r.datasets {
		if dataset.ProjectID == projectID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	items := make([]DatasetSummary, 0, len(ids))
	for _, datasetID := range ids {
		dataset := r.datasets[datasetID]
		summary := DatasetSummary{
			ID:            dataset.ID,
			ProjectID:     dataset.ProjectID,
			Name:          dataset.Name,
			Bucket:        dataset.Bucket,
			Prefix:        dataset.Prefix,
			ItemCount:     len(r.items[datasetID]),
			SnapshotCount: len(r.snapshots[datasetID]),
		}
		if count := len(r.snapshots[datasetID]); count > 0 {
			latest := r.snapshots[datasetID][count-1]
			summary.LatestSnapshotID = &latest.ID
			summary.LatestSnapshotVersion = latest.Version
		}
		items = append(items, summary)
	}
	return items, nil
}

func (r *InMemoryRepository) GetDatasetDetail(_ context.Context, datasetID int64) (DatasetDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dataset, ok := r.datasets[datasetID]
	if !ok {
		return DatasetDetail{}, wrapNotFound("dataset", datasetID)
	}

	out := DatasetDetail{
		ID:            dataset.ID,
		ProjectID:     dataset.ProjectID,
		Name:          dataset.Name,
		Bucket:        dataset.Bucket,
		Prefix:        dataset.Prefix,
		ItemCount:     len(r.items[datasetID]),
		SnapshotCount: len(r.snapshots[datasetID]),
	}
	if count := len(r.snapshots[datasetID]); count > 0 {
		latest := r.snapshots[datasetID][count-1]
		out.LatestSnapshotID = &latest.ID
		out.LatestSnapshotVersion = latest.Version
	}
	return out, nil
}

func (r *InMemoryRepository) GetSnapshotDetail(_ context.Context, snapshotID int64) (SnapshotDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for datasetID, snapshots := range r.snapshots {
		for _, snapshot := range snapshots {
			if snapshot.ID != snapshotID {
				continue
			}
			dataset := r.datasets[datasetID]
			annotationCount := 0
			for _, annotation := range r.annotations {
				if annotation.SnapshotID == snapshotID {
					annotationCount++
				}
			}
			return SnapshotDetail{
				ID:                snapshot.ID,
				DatasetID:         snapshot.DatasetID,
				DatasetName:       dataset.Name,
				ProjectID:         dataset.ProjectID,
				Version:           snapshot.Version,
				BasedOnSnapshotID: snapshot.BasedOnSnapshot,
				Note:              snapshot.Note,
				AnnotationCount:   annotationCount,
			}, nil
		}
	}
	return SnapshotDetail{}, wrapNotFound("snapshot", snapshotID)
}
```

Add the PostgreSQL browse queries to `internal/datahub/postgres_repository.go`:

```go
import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)
```

```go
func (r *PostgresRepository) ListDatasets(ctx context.Context, projectID int64) ([]DatasetSummary, error) {
	rows, err := r.pool.Query(ctx, `
		select
			d.id,
			d.project_id,
			d.name,
			d.bucket,
			d.prefix,
			coalesce(item_counts.item_count, 0)::int as item_count,
			coalesce(snapshot_counts.snapshot_count, 0)::int as snapshot_count,
			latest_snapshot.id,
			latest_snapshot.version
		from datasets d
		left join lateral (
			select count(*)::int as item_count
			from dataset_items di
			where di.dataset_id = d.id
		) item_counts on true
		left join lateral (
			select count(*)::int as snapshot_count
			from dataset_snapshots ds
			where ds.dataset_id = d.id
		) snapshot_counts on true
		left join lateral (
			select ds.id, ds.version
			from dataset_snapshots ds
			where ds.dataset_id = d.id
			order by ds.id desc
			limit 1
		) latest_snapshot on true
		where d.project_id = $1
		order by d.id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []DatasetSummary{}
	for rows.Next() {
		var item DatasetSummary
		var latestSnapshotID sql.NullInt64
		var latestSnapshotVersion sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.Name,
			&item.Bucket,
			&item.Prefix,
			&item.ItemCount,
			&item.SnapshotCount,
			&latestSnapshotID,
			&latestSnapshotVersion,
		); err != nil {
			return nil, err
		}
		if latestSnapshotID.Valid {
			id := latestSnapshotID.Int64
			item.LatestSnapshotID = &id
		}
		item.LatestSnapshotVersion = latestSnapshotVersion.String
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) GetDatasetDetail(ctx context.Context, datasetID int64) (DatasetDetail, error) {
	var out DatasetDetail
	var latestSnapshotID sql.NullInt64
	var latestSnapshotVersion sql.NullString
	err := r.pool.QueryRow(ctx, `
		select
			d.id,
			d.project_id,
			d.name,
			d.bucket,
			d.prefix,
			coalesce(item_counts.item_count, 0)::int as item_count,
			coalesce(snapshot_counts.snapshot_count, 0)::int as snapshot_count,
			latest_snapshot.id,
			latest_snapshot.version
		from datasets d
		left join lateral (
			select count(*)::int as item_count
			from dataset_items di
			where di.dataset_id = d.id
		) item_counts on true
		left join lateral (
			select count(*)::int as snapshot_count
			from dataset_snapshots ds
			where ds.dataset_id = d.id
		) snapshot_counts on true
		left join lateral (
			select ds.id, ds.version
			from dataset_snapshots ds
			where ds.dataset_id = d.id
			order by ds.id desc
			limit 1
		) latest_snapshot on true
		where d.id = $1
	`, datasetID).Scan(
		&out.ID,
		&out.ProjectID,
		&out.Name,
		&out.Bucket,
		&out.Prefix,
		&out.ItemCount,
		&out.SnapshotCount,
		&latestSnapshotID,
		&latestSnapshotVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DatasetDetail{}, wrapNotFound("dataset", datasetID)
		}
		return DatasetDetail{}, err
	}
	if latestSnapshotID.Valid {
		id := latestSnapshotID.Int64
		out.LatestSnapshotID = &id
	}
	out.LatestSnapshotVersion = latestSnapshotVersion.String
	return out, nil
}

func (r *PostgresRepository) GetSnapshotDetail(ctx context.Context, snapshotID int64) (SnapshotDetail, error) {
	var out SnapshotDetail
	err := r.pool.QueryRow(ctx, `
		select
			s.id,
			s.dataset_id,
			d.name,
			d.project_id,
			s.version,
			s.based_on_snapshot_id,
			coalesce(s.note, ''),
			coalesce(annotation_counts.annotation_count, 0)::int
		from dataset_snapshots s
		join datasets d on d.id = s.dataset_id
		left join lateral (
			select count(*)::int as annotation_count
			from annotations a
			where a.created_at_snapshot_id = s.id
			  and a.review_status = 'verified'
		) annotation_counts on true
		where s.id = $1
	`, snapshotID).Scan(
		&out.ID,
		&out.DatasetID,
		&out.DatasetName,
		&out.ProjectID,
		&out.Version,
		&out.BasedOnSnapshotID,
		&out.Note,
		&out.AnnotationCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SnapshotDetail{}, wrapNotFound("snapshot", snapshotID)
		}
		return SnapshotDetail{}, err
	}
	return out, nil
}
```

Update `internal/datahub/postgres_repository.go` to coalesce nullable snapshot notes on existing reads:

```go
func (r *PostgresRepository) GetSnapshot(ctx context.Context, snapshotID int64) (Snapshot, error) {
	var out Snapshot
	err := r.pool.QueryRow(ctx, `
		select id, dataset_id, version, based_on_snapshot_id, coalesce(note, '')
		from dataset_snapshots
		where id = $1
	`, snapshotID).Scan(&out.ID, &out.DatasetID, &out.Version, &out.BasedOnSnapshot, &out.Note)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, wrapNotFound("snapshot", snapshotID)
	}
	return out, err
}

func (r *PostgresRepository) ListSnapshots(ctx context.Context, datasetID int64) ([]Snapshot, error) {
	rows, err := r.pool.Query(ctx, `
		select id, dataset_id, version, based_on_snapshot_id, coalesce(note, '')
		from dataset_snapshots
		where dataset_id = $1
		order by id asc
	`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := []Snapshot{}
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.DatasetID, &snap.Version, &snap.BasedOnSnapshot, &snap.Note); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, rows.Err()
}
```

Add the service methods to `internal/datahub/service.go`:

```go
func (s *Service) ListDatasets(projectID int64) ([]DatasetSummary, error) {
	if projectID <= 0 {
		return nil, errors.New("project_id must be > 0")
	}
	return s.repo.ListDatasets(context.Background(), projectID)
}

func (s *Service) GetDatasetDetail(datasetID int64) (DatasetDetail, error) {
	if datasetID <= 0 {
		return DatasetDetail{}, errors.New("dataset_id must be > 0")
	}
	return s.repo.GetDatasetDetail(context.Background(), datasetID)
}

func (s *Service) GetSnapshotDetail(snapshotID int64) (SnapshotDetail, error) {
	if snapshotID <= 0 {
		return SnapshotDetail{}, errors.New("snapshot_id must be > 0")
	}
	return s.repo.GetSnapshotDetail(context.Background(), snapshotID)
}
```

- [ ] **Step 4: Run the datahub tests again**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub -count=1 -v
```

Expected: PASS for the new browse-model service tests and the existing datahub suite.

- [ ] **Step 5: Commit the repository/service slice**

Run:

```bash
git add internal/datahub/browse_models.go internal/datahub/errors.go internal/datahub/repository.go internal/datahub/postgres_repository.go internal/datahub/postgres_repository_test.go internal/datahub/service.go internal/datahub/service_test.go
git commit -m "feat: add datahub browse queries"
```

## Task 2: Expose Dataset And Snapshot Read Endpoints

**Files:**

- Modify: `internal/datahub/handler.go`
- Modify: `internal/datahub/handler_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`

- [ ] **Step 1: Write the failing HTTP and route tests**

Append these tests to `internal/datahub/handler_test.go`:

```go
func TestListDatasetsAndGetSnapshotDetail(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "browse-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train/browse",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/browse/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "baseline"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	srv := server.NewHTTPServerWithDataHub(datahub.NewHandler(svc))

	listReq := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	listRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list datasets failed: %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"name":"browse-dataset"`) {
		t.Fatalf("expected browse dataset in list response, got %s", listRec.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/snapshots/1", nil)
	detailRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("get snapshot detail failed: %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	if !strings.Contains(detailRec.Body.String(), `"dataset_name":"browse-dataset"`) {
		t.Fatalf("expected dataset_name in snapshot detail, got %s", detailRec.Body.String())
	}
	if !strings.Contains(detailRec.Body.String(), `"version":"`+snapshot.Version+`"`) {
		t.Fatalf("expected version in snapshot detail, got %s", detailRec.Body.String())
	}
}

func TestGetDatasetDetailReturns404WhenDatasetDoesNotExist(t *testing.T) {
	srv := server.NewHTTPServerWithDataHub(datahub.NewHandler(datahub.NewService(nil)))

	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/999", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSnapshotDetailRejectsInvalidSnapshotID(t *testing.T) {
	srv := server.NewHTTPServerWithDataHub(datahub.NewHandler(datahub.NewService(nil)))

	req := httptest.NewRequest(http.MethodGet, "/v1/snapshots/not-a-number", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
```

Update the route assertions in `internal/server/http_server_routes_test.go`:

```go
		{http.MethodGet, "/v1/datasets"},
		{http.MethodGet, "/v1/datasets/1"},
		{http.MethodGet, "/v1/snapshots/1"},
```

Append this assertion block to `cmd/api-server/main_test.go` inside `TestNewTestModulesLeavesTaskAndOverviewRoutesUnwired`:

```go
	datasetReq := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	datasetRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(datasetRec, datasetReq)
	if datasetRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for dataset browse route before datahub wiring, got %d", datasetRec.Code)
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/v1/snapshots/1", nil)
	snapshotRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for snapshot browse route before datahub wiring, got %d", snapshotRec.Code)
	}
```

- [ ] **Step 2: Run the HTTP tests to verify they fail**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub ./internal/server ./cmd/api-server -count=1 -v
```

Expected: FAIL because the new browse handlers and route wiring are not implemented yet.

- [ ] **Step 3: Implement the handlers and route wiring**

Update `internal/datahub/handler.go`:

```go
func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, ErrNotFound) {
		status = http.StatusNotFound
	}
	writeError(w, status, err)
}

func (h *Handler) ListDatasets(w http.ResponseWriter, _ *http.Request) {
	items, err := h.svc.ListDatasets(1)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetDatasetDetail(w http.ResponseWriter, r *http.Request) {
	datasetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	detail, err := h.svc.GetDatasetDetail(datasetID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) GetSnapshotDetail(w http.ResponseWriter, r *http.Request) {
	snapshotID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	detail, err := h.svc.GetSnapshotDetail(snapshotID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
```

Extend `internal/server/http_server.go`:

```go
type DataHubRoutes struct {
	ListDatasets           http.HandlerFunc
	CreateDataset          http.HandlerFunc
	GetDatasetDetail       http.HandlerFunc
	ScanDataset            http.HandlerFunc
	CreateSnapshot         http.HandlerFunc
	GetSnapshotDetail      http.HandlerFunc
	ListSnapshots          http.HandlerFunc
	ListItems              http.HandlerFunc
	PresignObject          http.HandlerFunc
	ImportSnapshot         http.HandlerFunc
	CompleteImportSnapshot http.HandlerFunc
}
```

Wire the routes in `internal/server/http_server.go`:

```go
	r.Route("/v1", func(r chi.Router) {
		r.Get("/datasets", handlerOrNotImplemented(m.DataHub.ListDatasets))
		r.Post("/datasets", handlerOrNotImplemented(m.DataHub.CreateDataset))
		r.Get("/datasets/{id}", handlerOrNotImplemented(m.DataHub.GetDatasetDetail))
		r.Post("/datasets/{id}/scan", handlerOrNotImplemented(m.DataHub.ScanDataset))
		r.Post("/datasets/{id}/snapshots", handlerOrNotImplemented(m.DataHub.CreateSnapshot))
		r.Get("/datasets/{id}/snapshots", handlerOrNotImplemented(m.DataHub.ListSnapshots))
		r.Get("/datasets/{id}/items", handlerOrNotImplemented(m.DataHub.ListItems))
		r.Get("/snapshots/{id}", handlerOrNotImplemented(m.DataHub.GetSnapshotDetail))
```

Wire the new handlers in `internal/server/http_server.go` constructors and in `cmd/api-server/main.go`:

```go
	dataHubRoutes = DataHubRoutes{
		ListDatasets:           dataHubHandler.ListDatasets,
		CreateDataset:          dataHubHandler.CreateDataset,
		GetDatasetDetail:       dataHubHandler.GetDatasetDetail,
		ScanDataset:            dataHubHandler.ScanDataset,
		CreateSnapshot:         dataHubHandler.CreateSnapshot,
		GetSnapshotDetail:      dataHubHandler.GetSnapshotDetail,
		ListSnapshots:          dataHubHandler.ListSnapshots,
		ListItems:              dataHubHandler.ListItems,
		PresignObject:          dataHubHandler.PresignObject,
		ImportSnapshot:         dataHubHandler.ImportSnapshot,
		CompleteImportSnapshot: dataHubHandler.CompleteImportSnapshot,
	}
```

Also update the final `modules.DataHub` assignment in `cmd/api-server/main.go`:

```go
	modules.DataHub = server.DataHubRoutes{
		ListDatasets:           dataHubHandler.ListDatasets,
		CreateDataset:          dataHubHandler.CreateDataset,
		GetDatasetDetail:       dataHubHandler.GetDatasetDetail,
		ScanDataset:            dataHubHandler.ScanDataset,
		CreateSnapshot:         dataHubHandler.CreateSnapshot,
		GetSnapshotDetail:      dataHubHandler.GetSnapshotDetail,
		ListSnapshots:          dataHubHandler.ListSnapshots,
		ListItems:              dataHubHandler.ListItems,
		PresignObject:          dataHubHandler.PresignObject,
		ImportSnapshot:         dataHubHandler.ImportSnapshot,
		CompleteImportSnapshot: dataHubHandler.CompleteImportSnapshot,
	}
```

- [ ] **Step 4: Run the HTTP-focused tests again**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub ./internal/server ./cmd/api-server -count=1 -v
```

Expected: PASS for the new browse handlers, route registration, and not-implemented route-surface assertions.

- [ ] **Step 5: Commit the API surface**

Run:

```bash
git add internal/datahub/handler.go internal/datahub/handler_test.go internal/server/http_server.go internal/server/http_server_routes_test.go cmd/api-server/main.go cmd/api-server/main_test.go
git commit -m "feat: expose data browse endpoints"
```

## Task 3: Add Data Navigation, Dataset List, And Dataset Detail Pages

**Files:**

- Create: `apps/web/src/features/data/api.ts`
- Create: `apps/web/src/features/data/dataset-list-page.tsx`
- Create: `apps/web/src/features/data/dataset-detail-page.tsx`
- Create: `apps/web/src/features/data/data-pages.test.tsx`
- Modify: `apps/web/src/app/layout/app-shell.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/styles.css`

- [ ] **Step 1: Write the failing frontend tests for navigation, dataset list, and dataset detail**

Create `apps/web/src/features/data/data-pages.test.tsx` with:

```tsx
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "../../app/layout/app-shell";
import { DatasetDetailPage } from "./dataset-detail-page";
import { DatasetListPage } from "./dataset-list-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

function renderDataRoute(initialEntry: string) {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false },
		},
	});

	return render(
		<QueryClientProvider client={client}>
			<MemoryRouter initialEntries={[initialEntry]}>
				<Routes>
					<Route path="/" element={<AppShell />}>
						<Route path="data" element={<DatasetListPage />} />
						<Route path="data/datasets/:datasetId" element={<DatasetDetailPage />} />
					</Route>
				</Routes>
			</MemoryRouter>
		</QueryClientProvider>,
	);
}

describe("Data pages", () => {
	beforeEach(() => {
		global.fetch = vi.fn();
	});

	afterEach(() => {
		cleanup();
		vi.restoreAllMocks();
		global.fetch = originalFetch;
	});

	it("renders Data navigation and dataset list results", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(
			jsonResponse({
				items: [
					{
						id: 5,
						project_id: 1,
						name: "dock-night",
						bucket: "platform-dev",
						prefix: "train/night",
						item_count: 42,
						snapshot_count: 3,
						latest_snapshot_id: 12,
						latest_snapshot_version: "v3",
					},
				],
			}),
		);

		renderDataRoute("/data");

		expect(screen.getByRole("link", { name: "Data" })).toHaveAttribute("href", "/data");
		expect(await screen.findByRole("heading", { name: "Dataset List" })).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "dock-night" })).toHaveAttribute("href", "/data/datasets/5");
		expect(screen.getByRole("link", { name: "v3" })).toHaveAttribute("href", "/data/snapshots/12");
		expect(screen.getByText(/42 items/i)).toBeInTheDocument();
		expect(fetchMock).toHaveBeenCalledWith("/v1/datasets", expect.any(Object));
	});

	it("renders the empty dataset state", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(jsonResponse({ items: [] }));

		renderDataRoute("/data");

		expect(await screen.findByText("No datasets are registered yet.")).toBeInTheDocument();
	});

	it("renders dataset detail snapshots and compare links", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock
			.mockResolvedValueOnce(
				jsonResponse({
					id: 5,
					project_id: 1,
					name: "dock-night",
					bucket: "platform-dev",
					prefix: "train/night",
					item_count: 2,
					snapshot_count: 2,
					latest_snapshot_id: 12,
					latest_snapshot_version: "v2",
				}),
			)
			.mockResolvedValueOnce(
				jsonResponse({
					items: [
						{ id: 1, dataset_id: 5, object_key: "train/night/a.jpg", etag: "etag-a" },
						{ id: 2, dataset_id: 5, object_key: "train/night/b.jpg", etag: "etag-b" },
					],
				}),
			)
			.mockResolvedValueOnce(
				jsonResponse({
					items: [
						{ id: 11, dataset_id: 5, version: "v1", note: "baseline" },
						{
							id: 12,
							dataset_id: 5,
							version: "v2",
							note: "qa relabel",
							based_on_snapshot_id: 11,
						},
					],
				}),
			);

		renderDataRoute("/data/datasets/5");

		expect(await screen.findByRole("heading", { name: "dock-night" })).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "v2" })).toHaveAttribute("href", "/data/snapshots/12");
		expect(screen.getByRole("link", { name: "Compare with previous" })).toHaveAttribute(
			"href",
			"/data/diff?before=11&after=12",
		);
		expect(screen.getByText("train/night/a.jpg")).toBeInTheDocument();
	});

	it("renders dataset detail not-found state", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(jsonResponse({ error: "dataset 999 not found" }, 404));

		renderDataRoute("/data/datasets/999");

		expect(await screen.findByRole("alert")).toHaveTextContent("dataset 999 not found");
	});
});
```

- [ ] **Step 2: Run the frontend tests to verify they fail**

Run:

```bash
cd apps/web && npm test -- src/features/data/data-pages.test.tsx
```

Expected: FAIL because the `data` feature modules, routes, and navigation entry do not exist yet.

- [ ] **Step 3: Implement the data API client, list/detail pages, and shell wiring**

Create `apps/web/src/features/data/api.ts`:

```ts
import { getJSON, postJSON } from "../shared/http";

export interface DatasetSummary {
  id: number;
  project_id: number;
  name: string;
  bucket: string;
  prefix: string;
  item_count: number;
  latest_snapshot_id?: number;
  latest_snapshot_version?: string;
  snapshot_count: number;
}

export interface DatasetDetail extends DatasetSummary {}

export interface DatasetItem {
  id: number;
  dataset_id: number;
  object_key: string;
  etag: string;
}

export interface SnapshotItem {
  id: number;
  dataset_id: number;
  version: string;
  based_on_snapshot_id?: number;
  note?: string;
}

export interface SnapshotDetail {
  id: number;
  dataset_id: number;
  dataset_name: string;
  project_id: number;
  version: string;
  based_on_snapshot_id?: number;
  note: string;
  annotation_count: number;
}

export interface SnapshotDiffResponse {
  adds: Array<{ item_id: number; category_id: number }>;
  removes: Array<{ item_id: number; category_id: number }>;
  updates: Array<{ item_id: number; category_id: number; iou: number }>;
  stats: {
    added_count: number;
    removed_count: number;
    updated_count: number;
  };
  compatibility_score: number;
}

export function listDatasets() {
  return getJSON<{ items: DatasetSummary[] }>("/v1/datasets");
}

export function getDatasetDetail(datasetId: string | number) {
  return getJSON<DatasetDetail>(`/v1/datasets/${datasetId}`);
}

export function listDatasetItems(datasetId: string | number) {
  return getJSON<{ items: DatasetItem[] }>(`/v1/datasets/${datasetId}/items`);
}

export function listDatasetSnapshots(datasetId: string | number) {
  return getJSON<{ items: SnapshotItem[] }>(`/v1/datasets/${datasetId}/snapshots`);
}

export function getSnapshotDetail(snapshotId: string | number) {
  return getJSON<SnapshotDetail>(`/v1/snapshots/${snapshotId}`);
}

export function diffSnapshots(beforeSnapshotId: string | number, afterSnapshotId: string | number) {
  return postJSON<SnapshotDiffResponse>("/v1/snapshots/diff", {
    before_snapshot_id: Number(beforeSnapshotId),
    after_snapshot_id: Number(afterSnapshotId),
  });
}
```

Create `apps/web/src/features/data/dataset-list-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { listDatasets } from "./api";

export function DatasetListPage() {
  const datasetsQuery = useQuery({
    queryKey: ["datasets", 1],
    queryFn: listDatasets,
  });

  return (
    <section className="page-stack">
      <header className="page-hero">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Dataset List</h1>
          <p className="page-summary">
            Browse dataset coverage, recent snapshots, and the path into version inspection.
          </p>
        </div>
        <div className="hero-meter">
          <span>Datasets</span>
          <strong>{datasetsQuery.data?.items.length ?? 0}</strong>
          <small>active data sources</small>
        </div>
      </header>

      {datasetsQuery.isLoading ? <p>Loading datasets.</p> : null}
      {datasetsQuery.isError ? <p role="alert">Failed to load datasets: {datasetsQuery.error.message}</p> : null}

      {datasetsQuery.data ? (
        datasetsQuery.data.items.length === 0 ? (
          <section className="panel">
            <div className="panel-header">
              <h2>No datasets</h2>
              <span>Project 1</span>
            </div>
            <p>No datasets are registered yet.</p>
          </section>
        ) : (
          <div className="stack-list">
            {datasetsQuery.data.items.map((dataset) => (
              <article className="panel stack-item data-card" key={dataset.id}>
                <div className="task-card__heading">
                  <strong>
                    <Link to={`/data/datasets/${dataset.id}`}>{dataset.name}</Link>
                  </strong>
                  <span>{dataset.snapshot_count} snapshots</span>
                </div>
                <span>{dataset.bucket}/{dataset.prefix}</span>
                <span>{dataset.item_count} items</span>
                {dataset.latest_snapshot_id && dataset.latest_snapshot_version ? (
                  <Link className="context-link" to={`/data/snapshots/${dataset.latest_snapshot_id}`}>
                    {dataset.latest_snapshot_version}
                  </Link>
                ) : (
                  <span>No snapshots yet</span>
                )}
              </article>
            ))}
          </div>
        )
      ) : null}
    </section>
  );
}
```

Create `apps/web/src/features/data/dataset-detail-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getDatasetDetail, listDatasetItems, listDatasetSnapshots } from "./api";

export function DatasetDetailPage() {
  const { datasetId = "0" } = useParams();

  const detailQuery = useQuery({
    queryKey: ["dataset", datasetId],
    queryFn: () => getDatasetDetail(datasetId),
  });
  const itemsQuery = useQuery({
    queryKey: ["dataset-items", datasetId],
    queryFn: () => listDatasetItems(datasetId),
    enabled: detailQuery.isSuccess,
  });
  const snapshotsQuery = useQuery({
    queryKey: ["dataset-snapshots", datasetId],
    queryFn: () => listDatasetSnapshots(datasetId),
    enabled: detailQuery.isSuccess,
  });

  if (detailQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Dataset Detail</h1>
        <p>Loading dataset.</p>
      </section>
    );
  }

  if (detailQuery.isError || !detailQuery.data) {
    return (
      <section className="page-stack">
        <h1>Dataset Detail</h1>
        <p role="alert">{detailQuery.error?.message ?? "Dataset not found."}</p>
      </section>
    );
  }

  const dataset = detailQuery.data;
  const previewItems = itemsQuery.data?.items.slice(0, 6) ?? [];
  const snapshots = snapshotsQuery.data?.items ?? [];

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Dataset {dataset.id}</p>
          <h1>{dataset.name}</h1>
          <p className="page-summary">
            Inspect stored objects and snapshot lineage before entering task or review flows.
          </p>
        </div>
        <div className="hero-meter">
          <span>Snapshots</span>
          <strong>{dataset.snapshot_count}</strong>
          <small>{dataset.item_count} indexed items</small>
        </div>
      </header>

      <div className="detail-grid">
        <section className="panel detail-meta">
          <div className="panel-header">
            <h2>Dataset summary</h2>
            <span>{dataset.bucket}</span>
          </div>
          <dl>
            <dt>Prefix</dt>
            <dd>{dataset.prefix}</dd>
            <dt>Items</dt>
            <dd>{dataset.item_count}</dd>
            <dt>Latest snapshot</dt>
            <dd>
              {dataset.latest_snapshot_id && dataset.latest_snapshot_version ? (
                <Link to={`/data/snapshots/${dataset.latest_snapshot_id}`}>{dataset.latest_snapshot_version}</Link>
              ) : (
                "No snapshots yet"
              )}
            </dd>
          </dl>
        </section>

        <section className="panel">
          <div className="panel-header">
            <h2>Item preview</h2>
            <span>{previewItems.length}</span>
          </div>
          {previewItems.length === 0 ? <p>No indexed items yet.</p> : null}
          <div className="stack-list">
            {previewItems.map((item) => (
              <article className="stack-item" key={item.id}>
                <strong>{item.object_key}</strong>
                <span>{item.etag || "no etag"}</span>
              </article>
            ))}
          </div>
        </section>
      </div>

      <section className="panel">
        <div className="panel-header">
          <h2>Snapshots</h2>
          <span>{snapshots.length}</span>
        </div>
        {snapshots.length === 0 ? <p>No snapshots recorded yet.</p> : null}
        <div className="stack-list">
          {snapshots.map((snapshot) => (
            <article className="stack-item" key={snapshot.id}>
              <strong>
                <Link to={`/data/snapshots/${snapshot.id}`}>{snapshot.version}</Link>
              </strong>
              <span>{snapshot.note || "No note"}</span>
              {snapshot.based_on_snapshot_id ? (
                <Link
                  className="context-link"
                  to={`/data/diff?before=${snapshot.based_on_snapshot_id}&after=${snapshot.id}`}
                >
                  Compare with previous
                </Link>
              ) : null}
            </article>
          ))}
        </div>
      </section>
    </section>
  );
}
```

Update `apps/web/src/app/layout/app-shell.tsx`:

```tsx
        <nav className="shell-nav" aria-label="Primary">
          <NavLink to="/">Overview</NavLink>
          <NavLink to="/tasks">Tasks</NavLink>
          <NavLink to="/data">Data</NavLink>
        </nav>
```

Update `apps/web/src/app/router.tsx`:

```tsx
import { DatasetDetailPage } from "../features/data/dataset-detail-page";
import { DatasetListPage } from "../features/data/dataset-list-page";

      { path: "tasks", element: <TaskListPage /> },
      { path: "tasks/:taskId", element: <TaskDetailPage /> },
      { path: "data", element: <DatasetListPage /> },
      { path: "data/datasets/:datasetId", element: <DatasetDetailPage /> },
```

Append these styles to `apps/web/src/app/styles.css`:

```css
.data-card {
  gap: 10px;
}

.context-link {
  text-decoration: underline;
  text-decoration-thickness: 1px;
  text-underline-offset: 0.14em;
}

.inline-note {
  margin: 0;
  color: rgba(23, 32, 51, 0.72);
}

.diff-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
}

.diff-stat {
  padding: 16px;
  border-radius: 16px;
  background: rgba(243, 236, 223, 0.72);
  border: 1px solid rgba(23, 32, 51, 0.08);
  display: grid;
  gap: 4px;
}
```

- [ ] **Step 4: Run the new dataset list/detail tests**

Run:

```bash
cd apps/web && npm test -- src/features/data/data-pages.test.tsx
```

Expected: PASS for Data navigation, dataset list rendering, and dataset detail compare-link coverage.

- [ ] **Step 5: Commit the dataset browsing UI**

Run:

```bash
git add apps/web/src/features/data/api.ts apps/web/src/features/data/dataset-list-page.tsx apps/web/src/features/data/dataset-detail-page.tsx apps/web/src/features/data/data-pages.test.tsx apps/web/src/app/layout/app-shell.tsx apps/web/src/app/router.tsx apps/web/src/app/styles.css
git commit -m "feat: add dataset browsing pages"
```

## Task 4: Add Snapshot Detail, Snapshot Diff, And Task Backlinks

**Files:**

- Create: `apps/web/src/features/data/snapshot-detail-page.tsx`
- Create: `apps/web/src/features/data/snapshot-diff-page.tsx`
- Modify: `apps/web/src/features/data/data-pages.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/features/tasks/task-detail-page.tsx`
- Modify: `apps/web/src/features/tasks/task-detail-page.test.tsx`
- Modify: `apps/web/src/app/styles.css`

- [ ] **Step 1: Write the failing tests for snapshot detail, diff, and task backlinks**

Append these tests to `apps/web/src/features/data/data-pages.test.tsx`:

```tsx
import { SnapshotDetailPage } from "./snapshot-detail-page";
import { SnapshotDiffPage } from "./snapshot-diff-page";

						<Route path="data/snapshots/:snapshotId" element={<SnapshotDetailPage />} />
						<Route path="data/diff" element={<SnapshotDiffPage />} />
```

```tsx
	it("renders snapshot detail metadata and compare link", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(
			jsonResponse({
				id: 12,
				dataset_id: 5,
				dataset_name: "dock-night",
				project_id: 1,
				version: "v2",
				based_on_snapshot_id: 11,
				note: "qa relabel",
				annotation_count: 17,
			}),
		);

		renderDataRoute("/data/snapshots/12");

		expect(await screen.findByRole("heading", { name: "v2" })).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "dock-night" })).toHaveAttribute("href", "/data/datasets/5");
		expect(screen.getByRole("link", { name: "Compare with previous" })).toHaveAttribute(
			"href",
			"/data/diff?before=11&after=12",
		);
		expect(screen.getByText(/publish status is not wired in this slice yet/i)).toBeInTheDocument();
	});

	it("renders snapshot detail not-found state", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(jsonResponse({ error: "snapshot 999 not found" }, 404));

		renderDataRoute("/data/snapshots/999");

		expect(await screen.findByRole("alert")).toHaveTextContent("snapshot 999 not found");
	});

	it("renders snapshot diff parameter errors before fetching", () => {
		renderDataRoute("/data/diff");

		expect(screen.getByRole("alert")).toHaveTextContent("Both before and after snapshot ids are required.");
	});

	it("renders snapshot diff stats and changes", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(
			jsonResponse({
				adds: [{ item_id: 101, category_id: 7 }],
				removes: [{ item_id: 88, category_id: 7 }],
				updates: [{ item_id: 99, category_id: 7, iou: 0.82 }],
				stats: {
					added_count: 1,
					removed_count: 1,
					updated_count: 1,
				},
				compatibility_score: 0.91,
			}),
		);

		renderDataRoute("/data/diff?before=11&after=12");

		expect(await screen.findByRole("heading", { name: "Snapshot Diff" })).toBeInTheDocument();
		expect(screen.getByText("1")).toBeInTheDocument();
		expect(screen.getByText(/0.91/)).toBeInTheDocument();
		expect(screen.getByText(/added · item 101/i)).toBeInTheDocument();
		expect(fetchMock).toHaveBeenCalledWith(
			"/v1/snapshots/diff",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify({
					before_snapshot_id: 11,
					after_snapshot_id: 12,
				}),
			}),
		);
	});

	it("renders an empty diff without a blank page", async () => {
		const fetchMock = vi.mocked(global.fetch);
		fetchMock.mockResolvedValue(
			jsonResponse({
				adds: [],
				removes: [],
				updates: [],
				stats: {
					added_count: 0,
					removed_count: 0,
					updated_count: 0,
				},
				compatibility_score: 1,
			}),
		);

		renderDataRoute("/data/diff?before=11&after=12");

		expect(await screen.findByRole("heading", { name: "Snapshot Diff" })).toBeInTheDocument();
		expect(screen.getByText("No annotation delta detected between these snapshots.")).toBeInTheDocument();
	});
```

Append this test to `apps/web/src/features/tasks/task-detail-page.test.tsx`:

```tsx
  it("links dataset and snapshot context back into the data pages", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        id: 4,
        project_id: 1,
        snapshot_id: 12,
        snapshot_version: "v7",
        dataset_id: 5,
        dataset_name: "dock-night",
        title: "Review lane 4",
        kind: "review",
        status: "blocked",
        priority: "high",
        assignee: "reviewer-2",
        blocker_reason: "waiting for schema update",
        last_activity_at: "2026-04-01T08:00:00Z",
        created_at: "2026-04-01T08:00:00Z",
        updated_at: "2026-04-01T08:00:00Z",
      }),
    );

    renderPage();

    expect(await screen.findByRole("link", { name: "dock-night" })).toHaveAttribute("href", "/data/datasets/5");
    expect(screen.getByRole("link", { name: "v7" })).toHaveAttribute("href", "/data/snapshots/12");
  });
```

- [ ] **Step 2: Run the snapshot and backlink tests to verify they fail**

Run:

```bash
cd apps/web && npm test -- src/features/data/data-pages.test.tsx src/features/tasks/task-detail-page.test.tsx
```

Expected: FAIL because the snapshot pages do not exist and task detail still renders plain text for dataset/snapshot context.

- [ ] **Step 3: Implement snapshot detail, diff, and task-detail links**

Create `apps/web/src/features/data/snapshot-detail-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getSnapshotDetail } from "./api";

export function SnapshotDetailPage() {
  const { snapshotId = "0" } = useParams();
  const snapshotQuery = useQuery({
    queryKey: ["snapshot", snapshotId],
    queryFn: () => getSnapshotDetail(snapshotId),
  });

  if (snapshotQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Snapshot Detail</h1>
        <p>Loading snapshot.</p>
      </section>
    );
  }

  if (snapshotQuery.isError || !snapshotQuery.data) {
    return (
      <section className="page-stack">
        <h1>Snapshot Detail</h1>
        <p role="alert">{snapshotQuery.error?.message ?? "Snapshot not found."}</p>
      </section>
    );
  }

  const snapshot = snapshotQuery.data;

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Snapshot {snapshot.id}</p>
          <h1>{snapshot.version}</h1>
          <p className="page-summary">Inspect snapshot metadata and open a diff against its parent snapshot.</p>
        </div>
        <div className="hero-meter">
          <span>Annotations</span>
          <strong>{snapshot.annotation_count}</strong>
          <small>verified rows imported for this snapshot</small>
        </div>
      </header>

      <div className="detail-grid">
        <section className="panel detail-meta">
          <div className="panel-header">
            <h2>Snapshot metadata</h2>
            <span>Project {snapshot.project_id}</span>
          </div>
          <dl>
            <dt>Dataset</dt>
            <dd>
              <Link to={`/data/datasets/${snapshot.dataset_id}`}>{snapshot.dataset_name}</Link>
            </dd>
            <dt>Note</dt>
            <dd>{snapshot.note || "No note"}</dd>
            <dt>Parent snapshot</dt>
            <dd>{snapshot.based_on_snapshot_id ? `#${snapshot.based_on_snapshot_id}` : "No parent"}</dd>
          </dl>
        </section>

        <section className="panel panel-accent">
          <div className="panel-header">
            <h2>Publish status</h2>
            <span>Not wired</span>
          </div>
          <p className="inline-note">
            Publish status is not wired in this slice yet. Use this page for metadata and diff inspection only.
          </p>
          {snapshot.based_on_snapshot_id ? (
            <Link
              className="context-link"
              to={`/data/diff?before=${snapshot.based_on_snapshot_id}&after=${snapshot.id}`}
            >
              Compare with previous
            </Link>
          ) : null}
        </section>
      </div>
    </section>
  );
}
```

Create `apps/web/src/features/data/snapshot-diff-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { diffSnapshots } from "./api";

export function SnapshotDiffPage() {
  const [searchParams] = useSearchParams();
  const before = searchParams.get("before");
  const after = searchParams.get("after");

  const diffQuery = useQuery({
    queryKey: ["snapshot-diff", before, after],
    queryFn: () => diffSnapshots(before as string, after as string),
    enabled: Boolean(before && after),
  });

  if (!before || !after) {
    return (
      <section className="page-stack">
        <h1>Snapshot Diff</h1>
        <p role="alert">Both before and after snapshot ids are required.</p>
      </section>
    );
  }

  if (diffQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Snapshot Diff</h1>
        <p>Loading diff.</p>
      </section>
    );
  }

  if (diffQuery.isError || !diffQuery.data) {
    return (
      <section className="page-stack">
        <h1>Snapshot Diff</h1>
        <p role="alert">{diffQuery.error?.message ?? "Failed to load diff."}</p>
      </section>
    );
  }

  const diff = diffQuery.data;
  const changeRows = [
    ...diff.adds.map((item) => ({ label: `Added · item ${item.item_id}`, value: `category ${item.category_id}` })),
    ...diff.removes.map((item) => ({ label: `Removed · item ${item.item_id}`, value: `category ${item.category_id}` })),
    ...diff.updates.map((item) => ({ label: `Updated · item ${item.item_id}`, value: `IOU ${item.iou.toFixed(2)}` })),
  ];

  return (
    <section className="page-stack">
      <header className="page-hero">
        <div>
          <p className="page-kicker">Compare snapshots</p>
          <h1>Snapshot Diff</h1>
          <p className="page-summary">
            Review the compatibility score and raw annotation deltas before promoting downstream work.
          </p>
        </div>
        <div className="hero-meter">
          <span>Compatibility</span>
          <strong>{diff.compatibility_score.toFixed(2)}</strong>
          <small>{before} → {after}</small>
        </div>
      </header>

      <div className="diff-stats">
        <article className="diff-stat">
          <span>Added</span>
          <strong>{diff.stats.added_count}</strong>
        </article>
        <article className="diff-stat">
          <span>Removed</span>
          <strong>{diff.stats.removed_count}</strong>
        </article>
        <article className="diff-stat">
          <span>Updated</span>
          <strong>{diff.stats.updated_count}</strong>
        </article>
      </div>

      <section className="panel">
        <div className="panel-header">
          <h2>Changes</h2>
          <span>{changeRows.length}</span>
        </div>
        {changeRows.length === 0 ? <p>No annotation delta detected between these snapshots.</p> : null}
        <div className="stack-list">
          {changeRows.map((row) => (
            <article className="stack-item" key={row.label}>
              <strong>{row.label}</strong>
              <span>{row.value}</span>
            </article>
          ))}
        </div>
      </section>
    </section>
  );
}
```

Update `apps/web/src/app/router.tsx`:

```tsx
import { SnapshotDetailPage } from "../features/data/snapshot-detail-page";
import { SnapshotDiffPage } from "../features/data/snapshot-diff-page";

      { path: "data/snapshots/:snapshotId", element: <SnapshotDetailPage /> },
      { path: "data/diff", element: <SnapshotDiffPage /> },
```

Update `apps/web/src/features/tasks/task-detail-page.tsx`:

```tsx
import { Link, useParams } from "react-router-dom";
```

```tsx
            <dt>Dataset</dt>
            <dd>
              {task.dataset_id && task.dataset_name ? (
                <Link className="context-link" to={`/data/datasets/${task.dataset_id}`}>
                  {task.dataset_name}
                </Link>
              ) : (
                task.dataset_name || "No dataset context"
              )}
            </dd>
            <dt>Snapshot</dt>
            <dd>
              {task.snapshot_id && task.snapshot_version ? (
                <Link className="context-link" to={`/data/snapshots/${task.snapshot_id}`}>
                  {task.snapshot_version}
                </Link>
              ) : (
                task.snapshot_version || "No snapshot bound"
              )}
            </dd>
```

Append the mobile-safe diff spacing to `apps/web/src/app/styles.css`:

```css
.diff-stat strong {
  font-size: 1.8rem;
}
```

- [ ] **Step 4: Run the snapshot, task-detail, and build verification**

Run:

```bash
cd apps/web && npm test -- src/features/data/data-pages.test.tsx src/features/tasks/task-detail-page.test.tsx
cd apps/web && npm run build
```

Expected: PASS for snapshot detail, snapshot diff, task backlinks, and the production build.

- [ ] **Step 5: Commit the snapshot browsing slice**

Run:

```bash
git add apps/web/src/features/data/snapshot-detail-page.tsx apps/web/src/features/data/snapshot-diff-page.tsx apps/web/src/features/data/data-pages.test.tsx apps/web/src/app/router.tsx apps/web/src/features/tasks/task-detail-page.tsx apps/web/src/features/tasks/task-detail-page.test.tsx apps/web/src/app/styles.css
git commit -m "feat: add snapshot browsing flow"
```

## Final Verification

- [ ] Run the backend verification:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub ./internal/server ./cmd/api-server -count=1 -v
```

Expected: PASS for browse repositories, handlers, and route wiring.

- [ ] Run the frontend verification:

```bash
cd apps/web && npm test
```

Expected: PASS for overview, tasks, and the new data browsing pages.

- [ ] Run the frontend production build:

```bash
cd apps/web && npm run build
```

Expected: PASS with a production bundle emitted to `apps/web/dist`.
