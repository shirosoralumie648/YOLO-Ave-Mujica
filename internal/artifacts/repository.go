package artifacts

import (
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	Create(a Artifact) (Artifact, error)
	Get(id int64) (Artifact, bool)
	FindByFormatVersion(format, version string) (Artifact, bool)
}

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]Artifact
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{nextID: 1, byID: make(map[int64]Artifact)}
}

func (r *InMemoryRepository) Create(a Artifact) (Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a.ID = r.nextID
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	r.nextID++
	r.byID[a.ID] = a
	return a, nil
}

func (r *InMemoryRepository) Get(id int64) (Artifact, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	return a, ok
}

func (r *InMemoryRepository) FindByFormatVersion(format, version string) (Artifact, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, artifact := range r.byID {
		if artifact.Format == format && artifact.Version == version {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func (r *InMemoryRepository) MustGet(id int64) (Artifact, error) {
	a, ok := r.Get(id)
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %d not found", id)
	}
	return a, nil
}
