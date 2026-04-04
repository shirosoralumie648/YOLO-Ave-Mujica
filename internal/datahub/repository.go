package datahub

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

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
	LookupCategory(ctx context.Context, projectID int64, categoryName string) (int64, error)
	CreateAnnotation(ctx context.Context, snapshotID, datasetID, itemID int64, objectKey string, categoryID int64, categoryName string, bboxX, bboxY, bboxW, bboxH float64) error
	RecordAnnotationChange(ctx context.Context, change AnnotationChange) error
}

type StoredAnnotation struct {
	SnapshotID   int64
	DatasetID    int64
	ItemID       int64
	ObjectKey    string
	CategoryID   int64
	CategoryName string
	BBoxX        float64
	BBoxY        float64
	BBoxW        float64
	BBoxH        float64
}

type AnnotationChangePayload struct {
	ObjectKey    string  `json:"object_key"`
	CategoryID   int64   `json:"category_id"`
	CategoryName string  `json:"category_name"`
	BBoxX        float64 `json:"bbox_x"`
	BBoxY        float64 `json:"bbox_y"`
	BBoxW        float64 `json:"bbox_w"`
	BBoxH        float64 `json:"bbox_h"`
}

type AnnotationChange struct {
	FromSnapshotID int64                    `json:"from_snapshot_id"`
	ToSnapshotID   int64                    `json:"to_snapshot_id"`
	ItemID         int64                    `json:"item_id"`
	ChangeType     string                   `json:"change_type"`
	Before         *AnnotationChangePayload `json:"before,omitempty"`
	After          *AnnotationChangePayload `json:"after,omitempty"`
}

type InMemoryRepository struct {
	mu            sync.Mutex
	nextDataset   int64
	nextSnapshot  int64
	nextItem      int64
	nextCategory  int64
	datasets      map[int64]Dataset
	snapshots     map[int64][]Snapshot
	items         map[int64][]DatasetItem
	categoryIDs   map[string]int64
	categoryNames map[int64]string
	annotations   []StoredAnnotation
	changes       []AnnotationChange
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextDataset:   1,
		nextSnapshot:  1,
		nextItem:      1,
		nextCategory:  1,
		datasets:      make(map[int64]Dataset),
		snapshots:     make(map[int64][]Snapshot),
		items:         make(map[int64][]DatasetItem),
		categoryIDs:   make(map[string]int64),
		categoryNames: make(map[int64]string),
		annotations:   []StoredAnnotation{},
		changes:       []AnnotationChange{},
	}
}

func (r *InMemoryRepository) CreateDataset(_ context.Context, in CreateDatasetInput) (Dataset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d := Dataset{
		ID:        r.nextDataset,
		ProjectID: in.ProjectID,
		Name:      in.Name,
		Bucket:    in.Bucket,
		Prefix:    in.Prefix,
	}
	r.nextDataset++
	r.datasets[d.ID] = d
	r.items[d.ID] = []DatasetItem{}
	return d, nil
}

func (r *InMemoryRepository) GetDataset(_ context.Context, datasetID int64) (Dataset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dataset, ok := r.datasets[datasetID]
	if !ok {
		return Dataset{}, fmt.Errorf("dataset %d not found", datasetID)
	}
	return dataset, nil
}

func (r *InMemoryRepository) ListDatasets(_ context.Context, projectID int64) ([]DatasetSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]int64, 0, len(r.datasets))
	for id := range r.datasets {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]DatasetSummary, 0)
	for _, id := range ids {
		dataset := r.datasets[id]
		if dataset.ProjectID != projectID {
			continue
		}
		summary := DatasetSummary{
			ID:            dataset.ID,
			ProjectID:     dataset.ProjectID,
			Name:          dataset.Name,
			Bucket:        dataset.Bucket,
			Prefix:        dataset.Prefix,
			ItemCount:     len(r.items[dataset.ID]),
			SnapshotCount: len(r.snapshots[dataset.ID]),
		}
		if n := len(r.snapshots[dataset.ID]); n > 0 {
			latest := r.snapshots[dataset.ID][n-1]
			latestID := latest.ID
			summary.LatestSnapshotID = &latestID
			summary.LatestSnapshotVersion = latest.Version
		}
		out = append(out, summary)
	}
	return out, nil
}

func (r *InMemoryRepository) GetDatasetDetail(_ context.Context, datasetID int64) (DatasetDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dataset, ok := r.datasets[datasetID]
	if !ok {
		return DatasetDetail{}, wrapNotFound("dataset", datasetID)
	}
	detail := DatasetDetail{
		ID:            dataset.ID,
		ProjectID:     dataset.ProjectID,
		Name:          dataset.Name,
		Bucket:        dataset.Bucket,
		Prefix:        dataset.Prefix,
		ItemCount:     len(r.items[dataset.ID]),
		SnapshotCount: len(r.snapshots[dataset.ID]),
	}
	if n := len(r.snapshots[dataset.ID]); n > 0 {
		latest := r.snapshots[dataset.ID][n-1]
		latestID := latest.ID
		detail.LatestSnapshotID = &latestID
		detail.LatestSnapshotVersion = latest.Version
	}
	return detail, nil
}

func (r *InMemoryRepository) CreateSnapshot(_ context.Context, datasetID int64, in CreateSnapshotInput) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.datasets[datasetID]; !ok {
		return Snapshot{}, fmt.Errorf("dataset %d not found", datasetID)
	}

	version := fmt.Sprintf("v%d", len(r.snapshots[datasetID])+1)
	s := Snapshot{
		ID:              r.nextSnapshot,
		DatasetID:       datasetID,
		Version:         version,
		BasedOnSnapshot: in.BasedOnSnapshotID,
		Note:            in.Note,
	}
	r.nextSnapshot++
	r.snapshots[datasetID] = append(r.snapshots[datasetID], s)
	return s, nil
}

func (r *InMemoryRepository) ListSnapshots(_ context.Context, datasetID int64) ([]Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.datasets[datasetID]; !ok {
		return nil, fmt.Errorf("dataset %d not found", datasetID)
	}
	out := make([]Snapshot, len(r.snapshots[datasetID]))
	copy(out, r.snapshots[datasetID])
	return out, nil
}

func (r *InMemoryRepository) GetSnapshot(_ context.Context, snapshotID int64) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for datasetID := range r.datasets {
		for _, snapshot := range r.snapshots[datasetID] {
			if snapshot.ID == snapshotID {
				return snapshot, nil
			}
		}
	}
	return Snapshot{}, fmt.Errorf("snapshot %d not found", snapshotID)
}

func (r *InMemoryRepository) GetSnapshotDetail(_ context.Context, snapshotID int64) (SnapshotDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found Snapshot
	foundSnapshot := false
	for _, snapshots := range r.snapshots {
		for _, snapshot := range snapshots {
			if snapshot.ID == snapshotID {
				found = snapshot
				foundSnapshot = true
				break
			}
		}
		if foundSnapshot {
			break
		}
	}
	if !foundSnapshot {
		return SnapshotDetail{}, wrapNotFound("snapshot", snapshotID)
	}

	dataset, ok := r.datasets[found.DatasetID]
	if !ok {
		return SnapshotDetail{}, wrapNotFound("dataset", found.DatasetID)
	}

	annotationCount := 0
	for _, annotation := range r.annotations {
		if annotation.DatasetID == found.DatasetID && annotation.SnapshotID <= snapshotID {
			annotationCount++
		}
	}

	return SnapshotDetail{
		ID:                found.ID,
		DatasetID:         found.DatasetID,
		Version:           found.Version,
		ProjectID:         dataset.ProjectID,
		DatasetName:       dataset.Name,
		BasedOnSnapshotID: found.BasedOnSnapshot,
		Note:              found.Note,
		AnnotationCount:   annotationCount,
	}, nil
}

func (r *InMemoryRepository) InsertItems(_ context.Context, datasetID int64, objectKeys []string) (int, error) {
	d, err := r.GetDataset(context.Background(), datasetID)
	if err != nil {
		return 0, err
	}
	if len(objectKeys) == 0 {
		objectKeys = []string{fmt.Sprintf("%s/sample-%d.jpg", d.Prefix, time.Now().UnixNano())}
	}

	objects := make([]ScannedObject, 0, len(objectKeys))
	for _, key := range objectKeys {
		if key == "" {
			continue
		}
		objects = append(objects, ScannedObject{Key: key})
	}
	return r.UpsertScannedItems(context.Background(), datasetID, objects)
}

func (r *InMemoryRepository) UpsertScannedItems(_ context.Context, datasetID int64, objects []ScannedObject) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.datasets[datasetID]; !ok {
		return 0, fmt.Errorf("dataset %d not found", datasetID)
	}

	existing := make(map[string]bool, len(r.items[datasetID]))
	for _, it := range r.items[datasetID] {
		existing[it.ObjectKey] = true
	}

	added := 0
	for _, object := range objects {
		if object.Key == "" || existing[object.Key] {
			continue
		}
		it := DatasetItem{
			ID:        r.nextItem,
			DatasetID: datasetID,
			ObjectKey: object.Key,
			ETag:      object.ETag,
		}
		if it.ETag == "" {
			it.ETag = fmt.Sprintf("etag-%d", r.nextItem)
		}
		r.nextItem++
		r.items[datasetID] = append(r.items[datasetID], it)
		existing[object.Key] = true
		added++
	}
	return added, nil
}

func (r *InMemoryRepository) ListItems(_ context.Context, datasetID int64) ([]DatasetItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.datasets[datasetID]; !ok {
		return nil, fmt.Errorf("dataset %d not found", datasetID)
	}
	out := make([]DatasetItem, len(r.items[datasetID]))
	copy(out, r.items[datasetID])
	return out, nil
}

func (r *InMemoryRepository) GetItemByObjectKey(_ context.Context, datasetID int64, objectKey string) (DatasetItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, item := range r.items[datasetID] {
		if item.ObjectKey == objectKey {
			return item, nil
		}
	}
	return DatasetItem{}, wrapNamedNotFound("dataset item", objectKey)
}

func (r *InMemoryRepository) EnsureCategory(_ context.Context, projectID int64, categoryName string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%d:%s", projectID, categoryName)
	if id, ok := r.categoryIDs[key]; ok {
		return id, nil
	}

	id := r.nextCategory
	r.nextCategory++
	r.categoryIDs[key] = id
	r.categoryNames[id] = categoryName
	return id, nil
}

func (r *InMemoryRepository) LookupCategory(_ context.Context, projectID int64, categoryName string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%d:%s", projectID, categoryName)
	if id, ok := r.categoryIDs[key]; ok {
		return id, nil
	}
	return 0, wrapNamedNotFound("category", categoryName)
}

func (r *InMemoryRepository) CreateAnnotation(_ context.Context, snapshotID, datasetID, itemID int64, objectKey string, categoryID int64, categoryName string, bboxX, bboxY, bboxW, bboxH float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.annotations = append(r.annotations, StoredAnnotation{
		SnapshotID:   snapshotID,
		DatasetID:    datasetID,
		ItemID:       itemID,
		ObjectKey:    objectKey,
		CategoryID:   categoryID,
		CategoryName: categoryName,
		BBoxX:        bboxX,
		BBoxY:        bboxY,
		BBoxW:        bboxW,
		BBoxH:        bboxH,
	})
	return nil
}

func (r *InMemoryRepository) RecordAnnotationChange(_ context.Context, change AnnotationChange) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, cloneAnnotationChange(change))
	return nil
}

func (r *InMemoryRepository) AnnotationsForSnapshot(snapshotID int64) []StoredAnnotation {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]StoredAnnotation, 0)
	for _, annotation := range r.annotations {
		if annotation.SnapshotID == snapshotID {
			out = append(out, annotation)
		}
	}
	return out
}

func (r *InMemoryRepository) AnnotationChangesForSnapshot(snapshotID int64) []AnnotationChange {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]AnnotationChange, 0)
	for _, change := range r.changes {
		if change.ToSnapshotID == snapshotID {
			out = append(out, cloneAnnotationChange(change))
		}
	}
	return out
}

func cloneAnnotationChange(change AnnotationChange) AnnotationChange {
	return AnnotationChange{
		FromSnapshotID: change.FromSnapshotID,
		ToSnapshotID:   change.ToSnapshotID,
		ItemID:         change.ItemID,
		ChangeType:     change.ChangeType,
		Before:         cloneAnnotationChangePayload(change.Before),
		After:          cloneAnnotationChangePayload(change.After),
	}
}

func cloneAnnotationChangePayload(payload *AnnotationChangePayload) *AnnotationChangePayload {
	if payload == nil {
		return nil
	}
	copy := *payload
	return &copy
}
