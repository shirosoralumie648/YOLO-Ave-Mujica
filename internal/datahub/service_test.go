package datahub

import (
	"reflect"
	"testing"
)

type fakeScanner struct {
	objects []ScannedObject
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
