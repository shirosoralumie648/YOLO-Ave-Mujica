package datahub

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeScanner struct {
	objects []ScannedObject
}

func mustSeedCategory(t *testing.T, repo *InMemoryRepository, projectID int64, categoryName string) int64 {
	t.Helper()

	categoryID, err := repo.EnsureCategory(context.Background(), projectID, categoryName)
	if err != nil {
		t.Fatalf("seed category %s: %v", categoryName, err)
	}
	return categoryID
}

func (s fakeScanner) ListObjects(bucket, prefix string) ([]ScannedObject, error) {
	return s.objects, nil
}

func TestScanDatasetUsesConfiguredObjectScanner(t *testing.T) {
	repo := NewInMemoryRepository()
	scanner := fakeScanner{
		objects: []ScannedObject{
			{Key: "train/a.jpg", ETag: "etag-a"},
			{Key: "train/b.jpg", ETag: "etag-b"},
		},
	}
	svc := NewServiceWithRepositoryAndScanner(nil, repo, scanner)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "scanner-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	added, err := svc.ScanDataset(dataset.ID, nil)
	if err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	if added != 2 {
		t.Fatalf("expected 2 indexed objects, got %d", added)
	}

	items, err := svc.ListItems(dataset.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 || items[0].ObjectKey != "train/a.jpg" || items[1].ObjectKey != "train/b.jpg" {
		t.Fatalf("unexpected scanned items: %+v", items)
	}
}

func TestScanDatasetPrefersExplicitObjectKeysOverScanner(t *testing.T) {
	repo := NewInMemoryRepository()
	scanner := fakeScanner{
		objects: []ScannedObject{},
	}
	svc := NewServiceWithRepositoryAndScanner(nil, repo, scanner)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "manual-scan-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	added, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"})
	if err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 indexed object from explicit keys, got %d", added)
	}

	items, err := svc.ListItems(dataset.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 || items[0].ObjectKey != "train/a.jpg" {
		t.Fatalf("unexpected scanned items: %+v", items)
	}
}

func TestGetSnapshotReturnsCreatedSnapshot(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "snapshot-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	created, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "baseline"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	got, err := svc.GetSnapshot(created.ID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if got.ID != created.ID || got.DatasetID != dataset.ID || got.Version != "v1" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestImportSnapshotCreatesCanonicalAnnotations(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")
	snapshot, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "import target"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	method := reflect.ValueOf(svc).MethodByName("ImportSnapshot")
	if !method.IsValid() {
		t.Fatal("ImportSnapshot method missing")
	}

	input := reflect.New(method.Type().In(1)).Elem()
	input.FieldByName("Format").SetString("yolo")
	input.FieldByName("SourceURI").SetString("s3://platform-dev/imports/yolo-demo.zip")

	entryType := input.FieldByName("Entries").Type().Elem()
	entry := reflect.New(entryType).Elem()
	entry.FieldByName("ObjectKey").SetString("train/a.jpg")
	entry.FieldByName("CategoryName").SetString("person")
	entry.FieldByName("BBoxX").SetFloat(0.1)
	entry.FieldByName("BBoxY").SetFloat(0.2)
	entry.FieldByName("BBoxW").SetFloat(0.3)
	entry.FieldByName("BBoxH").SetFloat(0.4)
	entries := reflect.MakeSlice(input.FieldByName("Entries").Type(), 1, 1)
	entries.Index(0).Set(entry)
	input.FieldByName("Entries").Set(entries)

	results := method.Call([]reflect.Value{reflect.ValueOf(snapshot.ID), input})
	if len(results) != 2 {
		t.Fatalf("expected ImportSnapshot to return 2 values, got %d", len(results))
	}
	if err, ok := results[1].Interface().(error); ok && err != nil {
		t.Fatalf("import snapshot: %v", err)
	}

	report := results[0]
	if got := report.FieldByName("DatasetID").Int(); got != dataset.ID {
		t.Fatalf("expected dataset id %d, got %d", dataset.ID, got)
	}
	if got := report.FieldByName("SnapshotID").Int(); got != snapshot.ID {
		t.Fatalf("expected snapshot id %d, got %d", snapshot.ID, got)
	}
	if got := report.FieldByName("ImportedAnnotations").Int(); got != 1 {
		t.Fatalf("expected 1 imported annotation, got %d", got)
	}

	repoMethod := reflect.ValueOf(repo).MethodByName("AnnotationsForSnapshot")
	if !repoMethod.IsValid() {
		t.Fatal("AnnotationsForSnapshot method missing")
	}
	annotations := repoMethod.Call([]reflect.Value{reflect.ValueOf(snapshot.ID)})
	if len(annotations) != 1 {
		t.Fatalf("expected 1 return value from AnnotationsForSnapshot, got %d", len(annotations))
	}
	if annotations[0].Len() != 1 {
		t.Fatalf("expected 1 stored annotation, got %d", annotations[0].Len())
	}

	stored := annotations[0].Index(0)
	if got := stored.FieldByName("ObjectKey").String(); got != "train/a.jpg" {
		t.Fatalf("expected stored object key train/a.jpg, got %s", got)
	}
	if got := stored.FieldByName("CategoryName").String(); got != "person" {
		t.Fatalf("expected stored category person, got %s", got)
	}
}

func TestImportSnapshotRecordsAnnotationChanges(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-change-log",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	item, err := repo.GetItemByObjectKey(context.Background(), dataset.ID, "train/a.jpg")
	if err != nil {
		t.Fatalf("lookup dataset item: %v", err)
	}
	parent, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "baseline"})
	if err != nil {
		t.Fatalf("create parent snapshot: %v", err)
	}
	child, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{
		BasedOnSnapshotID: &parent.ID,
		Note:              "import target",
	})
	if err != nil {
		t.Fatalf("create child snapshot: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")

	result, err := svc.ImportSnapshot(child.ID, ImportSnapshotInput{
		Format: "yolo",
		Entries: []ImportedAnnotation{{
			ObjectKey:    "train/a.jpg",
			CategoryName: "person",
			BBoxX:        0.1,
			BBoxY:        0.2,
			BBoxW:        0.3,
			BBoxH:        0.4,
		}},
	})
	if err != nil {
		t.Fatalf("import snapshot: %v", err)
	}
	if result.ImportedAnnotations != 1 {
		t.Fatalf("expected 1 imported annotation, got %d", result.ImportedAnnotations)
	}

	changes := repo.AnnotationChangesForSnapshot(child.ID)
	if len(changes) != 1 {
		t.Fatalf("expected 1 annotation change, got %d", len(changes))
	}
	change := changes[0]
	if change.FromSnapshotID != parent.ID {
		t.Fatalf("expected from_snapshot_id=%d, got %d", parent.ID, change.FromSnapshotID)
	}
	if change.ToSnapshotID != child.ID {
		t.Fatalf("expected to_snapshot_id=%d, got %d", child.ID, change.ToSnapshotID)
	}
	if change.ItemID != item.ID {
		t.Fatalf("expected item_id=%d, got %d", item.ID, change.ItemID)
	}
	if change.ChangeType != "added" {
		t.Fatalf("expected change_type=added, got %s", change.ChangeType)
	}
	if change.Before != nil {
		t.Fatalf("expected nil before payload for added annotation, got %+v", change.Before)
	}
	if change.After == nil {
		t.Fatal("expected after payload to be recorded")
	}
	if change.After.ObjectKey != "train/a.jpg" {
		t.Fatalf("expected after object key train/a.jpg, got %s", change.After.ObjectKey)
	}
	if change.After.CategoryName != "person" {
		t.Fatalf("expected after category person, got %s", change.After.CategoryName)
	}
	if change.After.BBoxW != 0.3 || change.After.BBoxH != 0.4 {
		t.Fatalf("unexpected after bbox payload: %+v", change.After)
	}
}

func findDatasetSummaryByID(items []DatasetSummary, datasetID int64) (DatasetSummary, bool) {
	for _, item := range items {
		if item.ID == datasetID {
			return item, true
		}
	}
	return DatasetSummary{}, false
}

func TestListDatasetsReturnsProjectScopedSummaries(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dockNight, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "dock-night",
		Bucket:    "platform-dev",
		Prefix:    "train/night",
	})
	if err != nil {
		t.Fatalf("create dock-night dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dockNight.ID, []string{"train/night/a.jpg", "train/night/b.jpg"}); err != nil {
		t.Fatalf("scan dock-night items: %v", err)
	}
	if _, err := svc.CreateSnapshot(dockNight.ID, CreateSnapshotInput{Note: "baseline"}); err != nil {
		t.Fatalf("create dock-night v1 snapshot: %v", err)
	}
	dockNightLatest, err := svc.CreateSnapshot(dockNight.ID, CreateSnapshotInput{Note: "relabel batch"})
	if err != nil {
		t.Fatalf("create dock-night v2 snapshot: %v", err)
	}

	yardDay, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "yard-day",
		Bucket:    "platform-dev",
		Prefix:    "train/day",
	})
	if err != nil {
		t.Fatalf("create yard-day dataset: %v", err)
	}
	if _, err := svc.ScanDataset(yardDay.ID, []string{"train/day/a.jpg"}); err != nil {
		t.Fatalf("scan yard-day item: %v", err)
	}
	yardDayParent, err := svc.CreateSnapshot(yardDay.ID, CreateSnapshotInput{Note: "seed"})
	if err != nil {
		t.Fatalf("create yard-day v1 snapshot: %v", err)
	}
	if _, err := svc.CreateSnapshot(yardDay.ID, CreateSnapshotInput{
		BasedOnSnapshotID: &yardDayParent.ID,
		Note:              "imported",
	}); err != nil {
		t.Fatalf("create yard-day v2 snapshot: %v", err)
	}

	otherProjectDataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 2,
		Name:      "ignore-me",
		Bucket:    "platform-dev",
		Prefix:    "train/other",
	})
	if err != nil {
		t.Fatalf("create project 2 dataset: %v", err)
	}
	if _, err := svc.ScanDataset(otherProjectDataset.ID, []string{"train/other/a.jpg"}); err != nil {
		t.Fatalf("scan project 2 dataset item: %v", err)
	}

	items, err := svc.ListDatasets(1)
	if err != nil {
		t.Fatalf("list datasets: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 datasets for project 1, got %d", len(items))
	}

	dockNightSummary, ok := findDatasetSummaryByID(items, dockNight.ID)
	if !ok {
		t.Fatalf("dock-night dataset %d not found in summaries: %+v", dockNight.ID, items)
	}
	if dockNightSummary.ItemCount != 2 {
		t.Fatalf("expected dock-night item_count=2, got %d", dockNightSummary.ItemCount)
	}
	if dockNightSummary.SnapshotCount != 2 {
		t.Fatalf("expected dock-night snapshot_count=2, got %d", dockNightSummary.SnapshotCount)
	}
	if dockNightSummary.LatestSnapshotID == nil || *dockNightSummary.LatestSnapshotID != dockNightLatest.ID {
		t.Fatalf("expected dock-night latest_snapshot_id=%d, got %+v", dockNightLatest.ID, dockNightSummary.LatestSnapshotID)
	}
	if dockNightSummary.LatestSnapshotVersion != "v2" {
		t.Fatalf("expected dock-night latest_snapshot_version=v2, got %s", dockNightSummary.LatestSnapshotVersion)
	}
}

func TestGetDatasetDetailReturnsAggregateCountsAndLatestSnapshot(t *testing.T) {
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
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/night/a.jpg", "train/night/b.jpg"}); err != nil {
		t.Fatalf("scan items: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "baseline"}); err != nil {
		t.Fatalf("create v1 snapshot: %v", err)
	}
	latest, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "relabel batch"})
	if err != nil {
		t.Fatalf("create v2 snapshot: %v", err)
	}

	detail, err := svc.GetDatasetDetail(dataset.ID)
	if err != nil {
		t.Fatalf("get dataset detail: %v", err)
	}
	if detail.ID != dataset.ID || detail.ProjectID != dataset.ProjectID || detail.Name != dataset.Name {
		t.Fatalf("unexpected dataset identity: %+v", detail)
	}
	if detail.ItemCount != 2 {
		t.Fatalf("expected item_count=2, got %d", detail.ItemCount)
	}
	if detail.SnapshotCount != 2 {
		t.Fatalf("expected snapshot_count=2, got %d", detail.SnapshotCount)
	}
	if detail.LatestSnapshotID == nil || *detail.LatestSnapshotID != latest.ID {
		t.Fatalf("expected latest_snapshot_id=%d, got %+v", latest.ID, detail.LatestSnapshotID)
	}
	if detail.LatestSnapshotVersion != "v2" {
		t.Fatalf("expected latest_snapshot_version=v2, got %s", detail.LatestSnapshotVersion)
	}
}

func TestGetSnapshotDetailReturnsDatasetAndAnnotationDetails(t *testing.T) {
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
		Note:              "relabel batch",
	})
	if err != nil {
		t.Fatalf("create child snapshot: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "car")
	if _, err := svc.ImportSnapshot(child.ID, ImportSnapshotInput{
		Format:    "yolo",
		SourceURI: "s3://platform-dev/imports/yard-day.zip",
		Entries: []ImportedAnnotation{
			{
				ObjectKey:    "train/day/a.jpg",
				CategoryName: "car",
				BBoxX:        0.1,
				BBoxY:        0.2,
				BBoxW:        0.3,
				BBoxH:        0.4,
			},
		},
	}); err != nil {
		t.Fatalf("import snapshot annotations: %v", err)
	}

	detail, err := svc.GetSnapshotDetail(child.ID)
	if err != nil {
		t.Fatalf("get snapshot detail: %v", err)
	}
	if detail.ID != child.ID || detail.DatasetID != dataset.ID {
		t.Fatalf("unexpected snapshot identity: %+v", detail)
	}
	if detail.ProjectID != dataset.ProjectID {
		t.Fatalf("expected project_id=%d, got %d", dataset.ProjectID, detail.ProjectID)
	}
	if detail.DatasetName != "yard-day" {
		t.Fatalf("expected dataset_name=yard-day, got %s", detail.DatasetName)
	}
	if detail.BasedOnSnapshotID == nil || *detail.BasedOnSnapshotID != parent.ID {
		t.Fatalf("expected based_on_snapshot_id=%d, got %+v", parent.ID, detail.BasedOnSnapshotID)
	}
	if detail.Note != "relabel batch" {
		t.Fatalf("expected note=relabel batch, got %s", detail.Note)
	}
	if detail.AnnotationCount != 1 {
		t.Fatalf("expected annotation_count=1, got %d", detail.AnnotationCount)
	}
}

func TestGetSnapshotDetailCountsEffectiveAnnotationsInheritedFromParent(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "yard-night",
		Bucket:    "platform-dev",
		Prefix:    "train/night",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/night/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}

	parent, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "seed"})
	if err != nil {
		t.Fatalf("create parent snapshot: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "car")
	if _, err := svc.ImportSnapshot(parent.ID, ImportSnapshotInput{
		Format: "yolo",
		Entries: []ImportedAnnotation{
			{
				ObjectKey:    "train/night/a.jpg",
				CategoryName: "car",
				BBoxX:        0.1,
				BBoxY:        0.2,
				BBoxW:        0.3,
				BBoxH:        0.4,
			},
		},
	}); err != nil {
		t.Fatalf("import parent annotations: %v", err)
	}

	child, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{
		BasedOnSnapshotID: &parent.ID,
		Note:              "review pass",
	})
	if err != nil {
		t.Fatalf("create child snapshot: %v", err)
	}

	detail, err := svc.GetSnapshotDetail(child.ID)
	if err != nil {
		t.Fatalf("get snapshot detail: %v", err)
	}
	if detail.AnnotationCount != 1 {
		t.Fatalf("expected inherited annotation_count=1, got %d", detail.AnnotationCount)
	}
}

func TestImportSnapshotRejectsUnsupportedFormat(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "unsupported-import-format",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	_, err = svc.ImportSnapshot(snapshot.ID, ImportSnapshotInput{
		Format: "pascal-voc",
		Entries: []ImportedAnnotation{{
			ObjectKey:    "train/a.jpg",
			CategoryName: "person",
			BBoxX:        1,
			BBoxY:        2,
			BBoxW:        3,
			BBoxH:        4,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestImportSnapshotRejectsDuplicateBoxes(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "duplicate-import-boxes",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")

	_, err = svc.ImportSnapshot(snapshot.ID, ImportSnapshotInput{
		Format: "yolo",
		Entries: []ImportedAnnotation{
			{
				ObjectKey:    "train/a.jpg",
				CategoryName: "person",
				BBoxX:        1,
				BBoxY:        2,
				BBoxW:        3,
				BBoxH:        4,
			},
			{
				ObjectKey:    "train/a.jpg",
				CategoryName: "person",
				BBoxX:        1,
				BBoxY:        2,
				BBoxW:        3,
				BBoxH:        4,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate annotation") {
		t.Fatalf("expected duplicate annotation error, got %v", err)
	}
}

func TestImportSnapshotRejectsUnknownCategory(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "unknown-category-import",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	_, err = svc.ImportSnapshot(snapshot.ID, ImportSnapshotInput{
		Format: "yolo",
		Entries: []ImportedAnnotation{{
			ObjectKey:    "train/a.jpg",
			CategoryName: "person",
			BBoxX:        0.1,
			BBoxY:        0.2,
			BBoxW:        0.3,
			BBoxH:        0.4,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown category") {
		t.Fatalf("expected unknown category error, got %v", err)
	}
}

func TestListDatasetsDatasetWithNoSnapshotsLeavesLatestSnapshotUnset(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(CreateDatasetInput{
		ProjectID: 1,
		Name:      "empty-snapshots",
		Bucket:    "platform-dev",
		Prefix:    "train/empty",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/empty/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}

	items, err := svc.ListDatasets(1)
	if err != nil {
		t.Fatalf("list datasets: %v", err)
	}
	summary, ok := findDatasetSummaryByID(items, dataset.ID)
	if !ok {
		t.Fatalf("dataset summary not found for dataset %d", dataset.ID)
	}
	if summary.SnapshotCount != 0 {
		t.Fatalf("expected snapshot_count=0, got %d", summary.SnapshotCount)
	}
	if summary.LatestSnapshotID != nil {
		t.Fatalf("expected latest_snapshot_id to be nil, got %+v", summary.LatestSnapshotID)
	}
	if summary.LatestSnapshotVersion != "" {
		t.Fatalf("expected latest_snapshot_version empty, got %q", summary.LatestSnapshotVersion)
	}

	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if strings.Contains(string(body), "\"latest_snapshot_id\"") {
		t.Fatalf("expected latest_snapshot_id field to be omitted when nil, body=%s", string(body))
	}

	detail, err := svc.GetDatasetDetail(dataset.ID)
	if err != nil {
		t.Fatalf("get dataset detail: %v", err)
	}
	if detail.LatestSnapshotID != nil {
		t.Fatalf("expected dataset detail latest_snapshot_id nil, got %+v", detail.LatestSnapshotID)
	}
}

func TestBrowseReadsReturnNotFoundErrors(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	if _, err := svc.GetDatasetDetail(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing dataset detail, got %v", err)
	}
	if _, err := svc.GetSnapshotDetail(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing snapshot detail, got %v", err)
	}
}

func TestBrowseMethodsRejectInvalidIdentifiers(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	if _, err := svc.ListDatasets(0); err == nil {
		t.Fatal("expected error for projectID <= 0")
	}
	if _, err := svc.GetDatasetDetail(0); err == nil {
		t.Fatal("expected error for datasetID <= 0")
	}
	if _, err := svc.GetSnapshotDetail(0); err == nil {
		t.Fatal("expected error for snapshotID <= 0")
	}
}

func TestSnapshotDetailJSONAlwaysIncludesNoteField(t *testing.T) {
	detail := SnapshotDetail{
		ID:              1,
		DatasetID:       1,
		Version:         "v1",
		ProjectID:       1,
		DatasetName:     "demo",
		AnnotationCount: 0,
		Note:            "",
	}

	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal snapshot detail: %v", err)
	}
	if !strings.Contains(string(body), "\"note\":\"\"") {
		t.Fatalf("expected note field in json, body=%s", string(body))
	}
}
