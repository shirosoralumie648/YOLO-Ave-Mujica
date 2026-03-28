package datahub

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	CreateDataset(ctx context.Context, in CreateDatasetInput) (Dataset, error)
	CreateSnapshot(ctx context.Context, datasetID int64, in CreateSnapshotInput) (Snapshot, error)
	ListSnapshots(ctx context.Context, datasetID int64) ([]Snapshot, error)
	InsertItems(ctx context.Context, datasetID int64, objectKeys []string) (int, error)
	ListItems(ctx context.Context, datasetID int64) ([]DatasetItem, error)
}

type InMemoryRepository struct {
	mu           sync.Mutex
	nextDataset  int64
	nextSnapshot int64
	nextItem     int64
	datasets     map[int64]Dataset
	snapshots    map[int64][]Snapshot
	items        map[int64][]DatasetItem
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextDataset:  1,
		nextSnapshot: 1,
		nextItem:     1,
		datasets:     make(map[int64]Dataset),
		snapshots:    make(map[int64][]Snapshot),
		items:        make(map[int64][]DatasetItem),
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

func (r *InMemoryRepository) InsertItems(_ context.Context, datasetID int64, objectKeys []string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.datasets[datasetID]
	if !ok {
		return 0, fmt.Errorf("dataset %d not found", datasetID)
	}

	if len(objectKeys) == 0 {
		objectKeys = []string{fmt.Sprintf("%s/sample-%d.jpg", d.Prefix, time.Now().UnixNano())}
	}

	existing := make(map[string]bool, len(r.items[datasetID]))
	for _, it := range r.items[datasetID] {
		existing[it.ObjectKey] = true
	}

	added := 0
	for _, key := range objectKeys {
		if key == "" || existing[key] {
			continue
		}
		it := DatasetItem{
			ID:        r.nextItem,
			DatasetID: datasetID,
			ObjectKey: key,
			ETag:      fmt.Sprintf("etag-%d", r.nextItem),
		}
		r.nextItem++
		r.items[datasetID] = append(r.items[datasetID], it)
		existing[key] = true
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
