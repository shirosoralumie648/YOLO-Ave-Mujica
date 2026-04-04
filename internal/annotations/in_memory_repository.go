package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type InMemoryRepository struct {
	mu          sync.Mutex
	nextID      int64
	byTaskID    map[int64]Annotation
	taskContext map[int64]AnnotationTaskContext
}

type AnnotationTaskContext struct {
	SnapshotID      int64
	AssetObjectKey  string
	FrameIndex      *int
	OntologyVersion string
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID:      1,
		byTaskID:    make(map[int64]Annotation),
		taskContext: make(map[int64]AnnotationTaskContext),
	}
}

func (r *InMemoryRepository) SeedTaskContext(taskID, snapshotID int64, assetObjectKey string, frameIndex *int, ontologyVersion string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.taskContext[taskID] = AnnotationTaskContext{
		SnapshotID:      snapshotID,
		AssetObjectKey:  assetObjectKey,
		FrameIndex:      cloneFrameIndex(frameIndex),
		OntologyVersion: ontologyVersion,
	}
}

func (r *InMemoryRepository) SaveDraft(_ context.Context, in SaveDraftInput) (Annotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	existing, ok := r.byTaskID[in.TaskID]
	ctx, hasContext := r.taskContext[in.TaskID]
	if !hasContext {
		return Annotation{}, newNotFoundError("task %d context not found", in.TaskID)
	}
	body, err := deepCloneJSONMap(in.Body)
	if err != nil {
		return Annotation{}, err
	}

	if !ok {
		if !sameAnnotationContextFromTaskContext(ctx, in) {
			return Annotation{}, newValidationError("annotation for task %d context mismatch", in.TaskID)
		}
		created := Annotation{
			ID:              r.nextID,
			TaskID:          in.TaskID,
			SnapshotID:      in.SnapshotID,
			AssetObjectKey:  in.AssetObjectKey,
			FrameIndex:      cloneFrameIndex(in.FrameIndex),
			OntologyVersion: in.OntologyVersion,
			State:           StateDraft,
			Revision:        1,
			Body:            body,
			SubmittedBy:     "",
			SubmittedAt:     nil,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		r.nextID++
		r.byTaskID[in.TaskID] = cloneAnnotation(created)
		return cloneAnnotation(created), nil
	}

	if in.BaseRevision > 0 && existing.Revision != in.BaseRevision {
		return Annotation{}, newConflictError("task %d revision mismatch: expected %d, got %d", in.TaskID, in.BaseRevision, existing.Revision)
	}
	if existing.State == StateSubmitted {
		return Annotation{}, newConflictError("annotation for task %d is already submitted", in.TaskID)
	}
	if !sameAnnotationContextInMemory(existing, in) || !sameAnnotationContextFromTaskContext(ctx, in) {
		return Annotation{}, newValidationError("annotation for task %d context mismatch", in.TaskID)
	}

	existing.State = StateDraft
	existing.Revision++
	existing.Body = body
	existing.SubmittedBy = ""
	existing.SubmittedAt = nil
	existing.UpdatedAt = now

	r.byTaskID[in.TaskID] = cloneAnnotation(existing)
	return cloneAnnotation(existing), nil
}

func (r *InMemoryRepository) Submit(_ context.Context, taskID int64, actor string) (Annotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.byTaskID[taskID]
	if !ok {
		return Annotation{}, newNotFoundError("annotation for task %d not found", taskID)
	}

	now := time.Now().UTC()
	if existing.State == StateSubmitted {
		return cloneAnnotation(existing), nil
	}
	existing.State = StateSubmitted
	existing.Revision++
	existing.SubmittedBy = actor
	existing.SubmittedAt = &now
	existing.UpdatedAt = now

	r.byTaskID[taskID] = cloneAnnotation(existing)
	return cloneAnnotation(existing), nil
}

func (r *InMemoryRepository) GetByTaskID(_ context.Context, taskID int64) (Annotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	annotation, ok := r.byTaskID[taskID]
	if !ok {
		return Annotation{}, newNotFoundError("annotation for task %d not found", taskID)
	}
	return cloneAnnotation(annotation), nil
}

func cloneAnnotation(in Annotation) Annotation {
	cloned, err := deepCloneJSONMap(in.Body)
	if err != nil {
		in.Body = map[string]any{}
	} else {
		in.Body = cloned
	}
	in.FrameIndex = cloneFrameIndex(in.FrameIndex)
	if in.SubmittedAt != nil {
		ts := *in.SubmittedAt
		in.SubmittedAt = &ts
	}
	return in
}

func deepCloneJSONMap(in map[string]any) (map[string]any, error) {
	if in == nil {
		return map[string]any{}, nil
	}

	raw, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encode body_json: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode body_json: %w", err)
	}
	if out == nil {
		return map[string]any{}, nil
	}
	return out, nil
}

func sameAnnotationContextInMemory(existing Annotation, in SaveDraftInput) bool {
	if existing.SnapshotID != in.SnapshotID {
		return false
	}
	if existing.AssetObjectKey != in.AssetObjectKey {
		return false
	}
	if existing.OntologyVersion != in.OntologyVersion {
		return false
	}
	if existing.FrameIndex == nil && in.FrameIndex == nil {
		return true
	}
	if existing.FrameIndex == nil || in.FrameIndex == nil {
		return false
	}
	return *existing.FrameIndex == *in.FrameIndex
}

func sameAnnotationContextFromTaskContext(ctx AnnotationTaskContext, in SaveDraftInput) bool {
	if ctx.SnapshotID != in.SnapshotID {
		return false
	}
	if ctx.AssetObjectKey != in.AssetObjectKey {
		return false
	}
	if ctx.OntologyVersion != in.OntologyVersion {
		return false
	}
	if ctx.FrameIndex == nil && in.FrameIndex == nil {
		return true
	}
	if ctx.FrameIndex == nil || in.FrameIndex == nil {
		return false
	}
	return *ctx.FrameIndex == *in.FrameIndex
}

func cloneFrameIndex(in *int) *int {
	if in == nil {
		return nil
	}
	value := *in
	return &value
}

var _ Repository = (*InMemoryRepository)(nil)
