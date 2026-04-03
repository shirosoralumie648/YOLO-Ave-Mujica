package annotations

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	if repo == nil {
		repo = errRepository{err: fmt.Errorf("annotations repository is required")}
	}
	return &Service{repo: repo}
}

func (s *Service) SaveDraft(ctx context.Context, in SaveDraftInput) (Annotation, error) {
	if in.TaskID <= 0 {
		return Annotation{}, fmt.Errorf("task_id must be > 0")
	}
	if in.SnapshotID <= 0 {
		return Annotation{}, fmt.Errorf("snapshot_id must be > 0")
	}
	in.Actor = normalizeActor(in.Actor)
	in.AssetObjectKey = strings.TrimSpace(in.AssetObjectKey)
	if in.AssetObjectKey == "" {
		return Annotation{}, fmt.Errorf("asset_object_key is required")
	}
	if in.FrameIndex != nil && *in.FrameIndex < 0 {
		return Annotation{}, fmt.Errorf("frame_index must be >= 0 when provided")
	}
	in.OntologyVersion = strings.TrimSpace(in.OntologyVersion)
	if in.OntologyVersion == "" {
		in.OntologyVersion = "v1"
	}
	if in.BaseRevision < 0 {
		return Annotation{}, fmt.Errorf("base_revision must be >= 0")
	}
	if in.Body == nil {
		in.Body = map[string]any{}
	}

	return s.repo.SaveDraft(ctx, in)
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) (Annotation, error) {
	if in.TaskID <= 0 {
		return Annotation{}, fmt.Errorf("task_id must be > 0")
	}
	return s.repo.Submit(ctx, in.TaskID, normalizeActor(in.Actor))
}

func (s *Service) GetByTaskID(ctx context.Context, taskID int64) (Annotation, error) {
	if taskID <= 0 {
		return Annotation{}, fmt.Errorf("task_id must be > 0")
	}
	return s.repo.GetByTaskID(ctx, taskID)
}

func normalizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "system"
	}
	return actor
}

type errRepository struct {
	err error
}

func (r errRepository) SaveDraft(_ context.Context, _ SaveDraftInput) (Annotation, error) {
	return Annotation{}, r.err
}

func (r errRepository) Submit(_ context.Context, _ int64, _ string) (Annotation, error) {
	return Annotation{}, r.err
}

func (r errRepository) GetByTaskID(_ context.Context, _ int64) (Annotation, error) {
	return Annotation{}, r.err
}
