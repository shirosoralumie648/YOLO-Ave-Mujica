package annotations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"yolo-ave-mujica/internal/tasks"
)

type Service struct {
	repo        Repository
	taskService TaskService
}

type TaskService interface {
	GetTask(ctx context.Context, taskID int64) (tasks.Task, error)
	TransitionTask(ctx context.Context, taskID int64, in tasks.TransitionTaskInput) (tasks.Task, error)
}

type taskContextSeeder interface {
	SeedTaskContext(taskID, snapshotID int64, assetObjectKey string, frameIndex *int, ontologyVersion string)
}

func NewService(repo Repository) *Service {
	return NewServiceWithTaskService(repo, nil)
}

func NewServiceWithTaskService(repo Repository, taskService TaskService) *Service {
	if repo == nil {
		repo = errRepository{err: fmt.Errorf("annotations repository is required")}
	}
	return &Service{repo: repo, taskService: taskService}
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

func (s *Service) GetWorkspace(ctx context.Context, taskID int64) (Workspace, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return Workspace{}, err
	}
	if err := validateWorkspaceTask(task); err != nil {
		return Workspace{}, err
	}

	annotation, err := s.repo.GetByTaskID(ctx, taskID)
	if err != nil {
		if isNotFoundError(err) {
			return s.buildWorkspace(task, syntheticDraft(task)), nil
		}
		return Workspace{}, err
	}
	return s.buildWorkspace(task, annotation), nil
}

func (s *Service) SaveWorkspaceDraft(ctx context.Context, taskID int64, in WorkspaceDraftInput) (Workspace, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return Workspace{}, err
	}
	if err := validateWorkspaceTask(task); err != nil {
		return Workspace{}, err
	}
	if err := ensureSnapshotContext(task); err != nil {
		return Workspace{}, err
	}

	if seeder, ok := s.repo.(taskContextSeeder); ok {
		seeder.SeedTaskContext(task.ID, *task.SnapshotID, task.AssetObjectKey, task.FrameIndex, workspaceOntologyVersion(task))
	}

	annotation, err := s.SaveDraft(ctx, SaveDraftInput{
		TaskID:          task.ID,
		Actor:           in.Actor,
		SnapshotID:      *task.SnapshotID,
		AssetObjectKey:  task.AssetObjectKey,
		FrameIndex:      cloneFrameIndex(task.FrameIndex),
		OntologyVersion: workspaceOntologyVersion(task),
		BaseRevision:    in.BaseRevision,
		Body:            in.Body,
	})
	if err != nil {
		return Workspace{}, err
	}
	return s.buildWorkspace(task, annotation), nil
}

func (s *Service) SubmitWorkspace(ctx context.Context, taskID int64, in SubmitInput) (Workspace, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return Workspace{}, err
	}
	if err := validateWorkspaceTask(task); err != nil {
		return Workspace{}, err
	}

	annotation, err := s.Submit(ctx, SubmitInput{
		TaskID: taskID,
		Actor:  in.Actor,
	})
	if err != nil {
		return Workspace{}, err
	}

	if task.Status != tasks.StatusSubmitted {
		task, err = s.taskService.TransitionTask(ctx, taskID, tasks.TransitionTaskInput{
			Status: tasks.StatusSubmitted,
		})
		if err != nil {
			return Workspace{}, err
		}
	}

	return s.buildWorkspace(task, annotation), nil
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

func (s *Service) getTask(ctx context.Context, taskID int64) (tasks.Task, error) {
	if taskID <= 0 {
		return tasks.Task{}, fmt.Errorf("task_id must be > 0")
	}
	if s.taskService == nil {
		return tasks.Task{}, fmt.Errorf("task service is required")
	}
	return s.taskService.GetTask(ctx, taskID)
}

func (s *Service) buildWorkspace(task tasks.Task, annotation Annotation) Workspace {
	return Workspace{
		Task: task,
		Asset: Asset{
			DatasetID:       task.DatasetID,
			DatasetName:     task.DatasetName,
			SnapshotID:      cloneSnapshotID(task.SnapshotID),
			SnapshotVersion: task.SnapshotVersion,
			ObjectKey:       task.AssetObjectKey,
			FrameIndex:      cloneFrameIndex(task.FrameIndex),
		},
		Draft: annotation,
	}
}

func syntheticDraft(task tasks.Task) Annotation {
	now := task.UpdatedAt
	if now.IsZero() {
		now = task.CreatedAt
	}

	return Annotation{
		TaskID:          task.ID,
		SnapshotID:      snapshotIDValue(task.SnapshotID),
		AssetObjectKey:  task.AssetObjectKey,
		FrameIndex:      cloneFrameIndex(task.FrameIndex),
		OntologyVersion: workspaceOntologyVersion(task),
		State:           StateDraft,
		Revision:        0,
		Body:            map[string]any{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func validateWorkspaceTask(task tasks.Task) error {
	if task.Kind != tasks.KindAnnotation {
		return fmt.Errorf("task %d is not an annotation task", task.ID)
	}
	if strings.TrimSpace(task.AssetObjectKey) == "" {
		return fmt.Errorf("task %d is missing asset_object_key", task.ID)
	}
	return nil
}

func ensureSnapshotContext(task tasks.Task) error {
	if task.SnapshotID == nil || *task.SnapshotID <= 0 {
		return fmt.Errorf("task %d is missing snapshot context", task.ID)
	}
	return nil
}

func workspaceOntologyVersion(task tasks.Task) string {
	version := strings.TrimSpace(task.OntologyVersion)
	if version == "" {
		return "v1"
	}
	return version
}

func cloneSnapshotID(in *int64) *int64 {
	if in == nil {
		return nil
	}
	value := *in
	return &value
}

func snapshotIDValue(in *int64) int64 {
	if in == nil {
		return 0
	}
	return *in
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, pgx.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "not found")
}
