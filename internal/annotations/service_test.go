package annotations

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"yolo-ave-mujica/internal/tasks"
)

func TestServiceSaveDraftIncrementsRevision(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(18, 7, "train/images/a.jpg", nil, "v1")
	svc := NewService(repo)

	first, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          18,
		Actor:           "annotator-1",
		SnapshotID:      7,
		AssetObjectKey:  "train/images/a.jpg",
		OntologyVersion: "v1",
		Body: map[string]any{
			"objects": []map[string]any{{"id": "box-1", "label": "person"}},
		},
	})
	if err != nil {
		t.Fatalf("save draft #1: %v", err)
	}

	second, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          18,
		Actor:           "annotator-1",
		SnapshotID:      7,
		AssetObjectKey:  "train/images/a.jpg",
		OntologyVersion: "v1",
		BaseRevision:    first.Revision,
		Body: map[string]any{
			"objects": []map[string]any{
				{"id": "box-1", "label": "person"},
				{"id": "box-2", "label": "car"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save draft #2: %v", err)
	}
	if second.Revision != first.Revision+1 {
		t.Fatalf("expected revision increment, got %d -> %d", first.Revision, second.Revision)
	}
}

func TestServiceSubmitMarksDraftSubmitted(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(18, 7, "train/images/a.jpg", nil, "v1")
	svc := NewService(repo)

	draft, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          18,
		Actor:           "annotator-1",
		SnapshotID:      7,
		AssetObjectKey:  "train/images/a.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	submitted, err := svc.Submit(ctx, SubmitInput{
		TaskID: draft.TaskID,
		Actor:  "annotator-1",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.State != StateSubmitted {
		t.Fatalf("expected submitted state, got %s", submitted.State)
	}
	if submitted.Revision != draft.Revision+1 {
		t.Fatalf("expected submit to increment revision, got %d -> %d", draft.Revision, submitted.Revision)
	}
}

func TestServiceSaveAfterSubmitDoesNotReopenAnnotation(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(33, 9, "train/images/c.jpg", nil, "v1")
	svc := NewService(repo)

	saved, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          33,
		Actor:           "annotator-1",
		SnapshotID:      9,
		AssetObjectKey:  "train/images/c.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	submitted, err := svc.Submit(ctx, SubmitInput{TaskID: saved.TaskID, Actor: "annotator-1"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.State != StateSubmitted {
		t.Fatalf("expected submitted state, got %s", submitted.State)
	}

	_, err = svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          saved.TaskID,
		Actor:           "annotator-1",
		SnapshotID:      9,
		AssetObjectKey:  "train/images/c.jpg",
		OntologyVersion: "v1",
		BaseRevision:    submitted.Revision,
		Body:            map[string]any{"objects": []any{map[string]any{"id": "box-1"}}},
	})
	if err == nil {
		t.Fatal("expected save after submit to fail")
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if !strings.Contains(err.Error(), "already submitted") {
		t.Fatalf("expected already submitted error, got %v", err)
	}
}

func TestServiceSaveDraftRejectsContextDrift(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(34, 11, "train/images/d.jpg", nil, "v1")
	svc := NewService(repo)

	initial, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          34,
		Actor:           "annotator-1",
		SnapshotID:      11,
		AssetObjectKey:  "train/images/d.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	_, err = svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          initial.TaskID,
		Actor:           "annotator-1",
		SnapshotID:      12,
		AssetObjectKey:  "train/images/other.jpg",
		OntologyVersion: "v2",
		BaseRevision:    initial.Revision,
		Body:            map[string]any{"objects": []any{}},
	})
	if err == nil {
		t.Fatal("expected context drift save to fail")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "context mismatch") {
		t.Fatalf("expected context mismatch error, got %v", err)
	}
}

func TestServiceSaveDraftRejectsRevisionMismatchAsConflict(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(40, 26, "train/images/j.jpg", nil, "v1")
	svc := NewService(repo)

	initial, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          40,
		Actor:           "annotator-1",
		SnapshotID:      26,
		AssetObjectKey:  "train/images/j.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	_, err = svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          initial.TaskID,
		Actor:           "annotator-1",
		SnapshotID:      26,
		AssetObjectKey:  "train/images/j.jpg",
		OntologyVersion: "v1",
		BaseRevision:    initial.Revision + 1,
		Body:            map[string]any{"objects": []any{}},
	})
	if err == nil {
		t.Fatal("expected revision mismatch save to fail")
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if !strings.Contains(err.Error(), "revision mismatch") {
		t.Fatalf("expected revision mismatch error, got %v", err)
	}
}

func TestServiceSaveDraftDeepCopiesNestedBody(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(35, 13, "train/images/e.jpg", nil, "v1")
	svc := NewService(repo)

	body := map[string]any{
		"objects": []any{
			map[string]any{
				"id":   "box-1",
				"meta": map[string]any{"label": "car"},
			},
		},
	}

	saved, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          35,
		Actor:           "annotator-1",
		SnapshotID:      13,
		AssetObjectKey:  "train/images/e.jpg",
		OntologyVersion: "v1",
		Body:            body,
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	objects := body["objects"].([]any)
	first := objects[0].(map[string]any)
	meta := first["meta"].(map[string]any)
	meta["label"] = "truck"
	body["objects"] = append(objects, map[string]any{"id": "box-2"})

	got, err := svc.GetByTaskID(ctx, saved.TaskID)
	if err != nil {
		t.Fatalf("get by task id: %v", err)
	}

	expected := map[string]any{
		"objects": []any{
			map[string]any{
				"id":   "box-1",
				"meta": map[string]any{"label": "car"},
			},
		},
	}
	if !reflect.DeepEqual(got.Body, expected) {
		t.Fatalf("expected stored body to stay unchanged, got %#v", got.Body)
	}
}

func TestServiceSaveDraftRejectsInitialContextMismatch(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(36, 21, "train/images/f.jpg", nil, "v1")
	svc := NewService(repo)

	_, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          36,
		Actor:           "annotator-1",
		SnapshotID:      22,
		AssetObjectKey:  "train/images/wrong.jpg",
		OntologyVersion: "v2",
		Body:            map[string]any{"objects": []any{}},
	})
	if err == nil {
		t.Fatal("expected initial save with mismatched task context to fail")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "context mismatch") {
		t.Fatalf("expected context mismatch error, got %v", err)
	}
}

func TestServiceSubmitIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(37, 23, "train/images/g.jpg", nil, "v1")
	svc := NewService(repo)

	draft, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          37,
		Actor:           "annotator-1",
		SnapshotID:      23,
		AssetObjectKey:  "train/images/g.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	first, err := svc.Submit(ctx, SubmitInput{TaskID: draft.TaskID, Actor: "annotator-1"})
	if err != nil {
		t.Fatalf("submit #1: %v", err)
	}
	second, err := svc.Submit(ctx, SubmitInput{TaskID: draft.TaskID, Actor: "annotator-2"})
	if err != nil {
		t.Fatalf("submit #2: %v", err)
	}

	if second.Revision != first.Revision {
		t.Fatalf("expected idempotent submit revision, got %d -> %d", first.Revision, second.Revision)
	}
	if second.SubmittedBy != first.SubmittedBy {
		t.Fatalf("expected submitted_by unchanged, got %q -> %q", first.SubmittedBy, second.SubmittedBy)
	}
	if first.SubmittedAt == nil || second.SubmittedAt == nil {
		t.Fatal("expected submitted_at to be set")
	}
	if !first.SubmittedAt.Equal(*second.SubmittedAt) {
		t.Fatalf("expected submitted_at unchanged, got %s -> %s", first.SubmittedAt.Format(time.RFC3339Nano), second.SubmittedAt.Format(time.RFC3339Nano))
	}
}

func TestServiceWithNilRepositoryFailsClearly(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	_, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          38,
		Actor:           "annotator-1",
		SnapshotID:      24,
		AssetObjectKey:  "train/images/h.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err == nil {
		t.Fatal("expected save draft to fail when repository is nil")
	}
	if !strings.Contains(err.Error(), "repository is required") {
		t.Fatalf("expected clear repository error, got %v", err)
	}
}

func TestServiceSaveDraftRejectsInvalidBody(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SeedTaskContext(39, 25, "train/images/i.jpg", nil, "v1")
	svc := NewService(repo)

	_, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          39,
		Actor:           "annotator-1",
		SnapshotID:      25,
		AssetObjectKey:  "train/images/i.jpg",
		OntologyVersion: "v1",
		Body: map[string]any{
			"invalid": func() {},
		},
	})
	if err == nil {
		t.Fatal("expected invalid body to fail")
	}
	if !strings.Contains(err.Error(), "body_json") {
		t.Fatalf("expected body_json error, got %v", err)
	}
}

func TestServiceGetWorkspaceReturnsTaskAndSyntheticDraft(t *testing.T) {
	ctx := context.Background()
	taskRepo := tasks.NewInMemoryRepository()
	taskSvc := tasks.NewService(taskRepo)
	snapshotID := int64(27)

	task, err := taskSvc.CreateTask(ctx, tasks.CreateTaskInput{
		ProjectID:       1,
		SnapshotID:      &snapshotID,
		Title:           "Annotate dock frame",
		Kind:            tasks.KindAnnotation,
		Status:          tasks.StatusInProgress,
		AssetObjectKey:  "train/images/workspace-a.jpg",
		MediaKind:       tasks.MediaKindImage,
		OntologyVersion: "v1",
		Assignee:        "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	task.SnapshotVersion = "v27"
	task.DatasetID = 8
	task.DatasetName = "yard-ops"

	svc := NewServiceWithTaskService(NewInMemoryRepository(), taskSvc)
	workspace, err := svc.GetWorkspace(ctx, task.ID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	if workspace.Task.ID != task.ID {
		t.Fatalf("expected task id %d, got %d", task.ID, workspace.Task.ID)
	}
	if workspace.Asset.ObjectKey != task.AssetObjectKey {
		t.Fatalf("expected object key %q, got %q", task.AssetObjectKey, workspace.Asset.ObjectKey)
	}
	if workspace.Asset.SnapshotID == nil || *workspace.Asset.SnapshotID != snapshotID {
		t.Fatalf("expected snapshot id %d, got %+v", snapshotID, workspace.Asset.SnapshotID)
	}
	if workspace.Draft.TaskID != task.ID {
		t.Fatalf("expected synthetic draft task id %d, got %d", task.ID, workspace.Draft.TaskID)
	}
	if workspace.Draft.State != StateDraft {
		t.Fatalf("expected synthetic draft state %q, got %q", StateDraft, workspace.Draft.State)
	}
	if workspace.Draft.Revision != 0 {
		t.Fatalf("expected synthetic draft revision 0, got %d", workspace.Draft.Revision)
	}
	if len(workspace.Draft.Body) != 0 {
		t.Fatalf("expected empty synthetic draft body, got %#v", workspace.Draft.Body)
	}
}

func TestServiceSaveWorkspaceDraftUsesTaskContext(t *testing.T) {
	ctx := context.Background()
	taskRepo := tasks.NewInMemoryRepository()
	taskSvc := tasks.NewService(taskRepo)
	snapshotID := int64(28)

	task, err := taskSvc.CreateTask(ctx, tasks.CreateTaskInput{
		ProjectID:       1,
		SnapshotID:      &snapshotID,
		Title:           "Annotate dock frame B",
		Kind:            tasks.KindAnnotation,
		Status:          tasks.StatusInProgress,
		AssetObjectKey:  "train/images/workspace-b.jpg",
		MediaKind:       tasks.MediaKindImage,
		OntologyVersion: "v2",
		Assignee:        "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	repo := NewInMemoryRepository()
	svc := NewServiceWithTaskService(repo, taskSvc)
	workspace, err := svc.SaveWorkspaceDraft(ctx, task.ID, WorkspaceDraftInput{
		Actor: "annotator-1",
		Body: map[string]any{
			"objects": []any{map[string]any{"id": "box-1", "label": "person"}},
		},
	})
	if err != nil {
		t.Fatalf("save workspace draft: %v", err)
	}

	if workspace.Draft.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", workspace.Draft.Revision)
	}
	if workspace.Draft.SnapshotID != snapshotID {
		t.Fatalf("expected draft snapshot id %d, got %d", snapshotID, workspace.Draft.SnapshotID)
	}
	if workspace.Draft.AssetObjectKey != task.AssetObjectKey {
		t.Fatalf("expected draft object key %q, got %q", task.AssetObjectKey, workspace.Draft.AssetObjectKey)
	}
	if workspace.Draft.OntologyVersion != task.OntologyVersion {
		t.Fatalf("expected ontology version %q, got %q", task.OntologyVersion, workspace.Draft.OntologyVersion)
	}
}

func TestServiceSubmitWorkspaceTransitionsTaskToSubmitted(t *testing.T) {
	ctx := context.Background()
	taskRepo := tasks.NewInMemoryRepository()
	taskSvc := tasks.NewService(taskRepo)
	snapshotID := int64(29)

	task, err := taskSvc.CreateTask(ctx, tasks.CreateTaskInput{
		ProjectID:       1,
		SnapshotID:      &snapshotID,
		Title:           "Annotate dock frame C",
		Kind:            tasks.KindAnnotation,
		Status:          tasks.StatusInProgress,
		AssetObjectKey:  "train/images/workspace-c.jpg",
		MediaKind:       tasks.MediaKindImage,
		OntologyVersion: "v1",
		Assignee:        "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	repo := NewInMemoryRepository()
	svc := NewServiceWithTaskService(repo, taskSvc)
	if _, err := svc.SaveWorkspaceDraft(ctx, task.ID, WorkspaceDraftInput{
		Actor: "annotator-1",
		Body:  map[string]any{"objects": []any{}},
	}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	workspace, err := svc.SubmitWorkspace(ctx, task.ID, SubmitInput{Actor: "annotator-1"})
	if err != nil {
		t.Fatalf("submit workspace: %v", err)
	}

	if workspace.Task.Status != tasks.StatusSubmitted {
		t.Fatalf("expected task status %q, got %q", tasks.StatusSubmitted, workspace.Task.Status)
	}
	if workspace.Draft.State != StateSubmitted {
		t.Fatalf("expected draft state %q, got %q", StateSubmitted, workspace.Draft.State)
	}
	currentTask, err := taskSvc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task after submit: %v", err)
	}
	if currentTask.Status != tasks.StatusSubmitted {
		t.Fatalf("expected persisted task status %q, got %q", tasks.StatusSubmitted, currentTask.Status)
	}
}
