package tasks

import (
	"context"
	"strings"
	"testing"
)

func TestServiceCreateListGetRoundTripAppliesDefaults(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID:      1,
		Title:          "Label frame 0001",
		AssetObjectKey: "assets/frame-0001.jpg",
		MediaKind:      MediaKindImage,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if created.Kind != KindAnnotation {
		t.Fatalf("expected default kind %q, got %q", KindAnnotation, created.Kind)
	}
	if created.Status != StatusQueued {
		t.Fatalf("expected default status %q, got %q", StatusQueued, created.Status)
	}
	if created.Priority != PriorityNormal {
		t.Fatalf("expected default priority %q, got %q", PriorityNormal, created.Priority)
	}
	if created.Assignee != "" {
		t.Fatalf("expected default empty assignee, got %q", created.Assignee)
	}
	if created.BlockerReason != "" {
		t.Fatalf("expected default empty blocker_reason, got %q", created.BlockerReason)
	}
	if created.LastActivityAt.IsZero() {
		t.Fatal("expected last_activity_at to be set")
	}
	if created.SnapshotID != nil {
		t.Fatal("expected snapshot_id to be nil when not provided")
	}

	listed, err := svc.ListTasks(ctx, 1, ListTasksFilter{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 task, got %d", len(listed))
	}
	if listed[0].ID != created.ID {
		t.Fatalf("unexpected listed task id: got %d want %d", listed[0].ID, created.ID)
	}

	got, err := svc.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("unexpected get id: got %d want %d", got.ID, created.ID)
	}
	if got.Status != StatusQueued {
		t.Fatalf("expected queued status on get, got %q", got.Status)
	}
}

func TestServiceCreateTaskBlockedRequiresBlockerReason(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	_, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Needs unblock",
		Kind:      KindReview,
		Status:    StatusBlocked,
	})
	if err == nil {
		t.Fatal("expected validation error for blocked task without blocker_reason")
	}
	if !strings.Contains(err.Error(), "blocker_reason") {
		t.Fatalf("expected blocker_reason error, got %v", err)
	}
}

func TestInMemoryRepositoryCreateTaskSetsLastActivityAtWhenZero(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()

	created, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "repo direct",
		Kind:      KindAnnotation,
		Status:    StatusQueued,
		Priority:  PriorityNormal,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if created.LastActivityAt.IsZero() {
		t.Fatal("expected last_activity_at to be non-zero")
	}
}

func TestServiceTransitionTaskFollowsAllowedStatusPath(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Review lane 4",
		Kind:      KindReview,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	ready, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusReady})
	if err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if ready.Status != StatusReady {
		t.Fatalf("expected ready, got %q", ready.Status)
	}

	inProgress, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress})
	if err != nil {
		t.Fatalf("transition to in_progress: %v", err)
	}
	if inProgress.Status != StatusInProgress {
		t.Fatalf("expected in_progress, got %q", inProgress.Status)
	}

	blocked, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{
		Status:        StatusBlocked,
		BlockerReason: "waiting for schema update",
	})
	if err != nil {
		t.Fatalf("transition to blocked: %v", err)
	}
	if blocked.Status != StatusBlocked || blocked.BlockerReason != "waiting for schema update" {
		t.Fatalf("unexpected blocked task: %+v", blocked)
	}

	resumed, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress})
	if err != nil {
		t.Fatalf("resume task: %v", err)
	}
	if resumed.Status != StatusInProgress {
		t.Fatalf("expected resumed in_progress, got %q", resumed.Status)
	}
	if resumed.BlockerReason != "" {
		t.Fatalf("expected blocker_reason to clear on resume, got %q", resumed.BlockerReason)
	}

	submitted, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusSubmitted})
	if err != nil {
		t.Fatalf("transition to submitted: %v", err)
	}
	if submitted.Status != StatusSubmitted {
		t.Fatalf("expected submitted, got %q", submitted.Status)
	}
}

func TestServiceTransitionTaskRejectsInvalidTransitionsAndBlockedWithoutReason(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Annotate night dock",
		Kind:      KindReview,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusSubmitted}); err == nil {
		t.Fatal("expected queued -> submitted transition to fail")
	}

	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusBlocked}); err == nil {
		t.Fatal("expected blocked transition without blocker_reason to fail")
	}
}

func TestServiceTransitionTaskSupportsSubmittedLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID:      1,
		Title:          "Annotation lifecycle",
		Kind:           KindAnnotation,
		AssetObjectKey: "assets/clip-01/frame-0005.jpg",
		MediaKind:      MediaKindImage,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	ready, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusReady})
	if err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if ready.Status != StatusReady {
		t.Fatalf("expected ready, got %q", ready.Status)
	}

	inProgress, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress})
	if err != nil {
		t.Fatalf("transition to in_progress: %v", err)
	}
	if inProgress.Status != StatusInProgress {
		t.Fatalf("expected in_progress, got %q", inProgress.Status)
	}

	submitted, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusSubmitted})
	if err != nil {
		t.Fatalf("transition to submitted: %v", err)
	}
	if submitted.Status != StatusSubmitted {
		t.Fatalf("expected submitted, got %q", submitted.Status)
	}
}

func TestServiceCreateTaskRequiresAssetContextForAnnotationTasks(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	_, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Missing asset context",
		Kind:      KindAnnotation,
	})
	if err == nil {
		t.Fatal("expected validation error for annotation task without asset context")
	}
}

func TestServiceTransitionTaskSupportsLegacyDoneCompatibility(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Legacy done path",
		Kind:      KindReview,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusReady}); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress}); err != nil {
		t.Fatalf("transition to in_progress: %v", err)
	}

	closed, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: "done"})
	if err != nil {
		t.Fatalf("transition to legacy done: %v", err)
	}
	if closed.Status != StatusClosed {
		t.Fatalf("expected legacy done to resolve to %q, got %q", StatusClosed, closed.Status)
	}
}

func TestServiceTransitionTaskMapsLegacyDoneToSubmittedForAnnotationTasks(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID:      1,
		Title:          "Annotation done compatibility",
		Kind:           KindAnnotation,
		AssetObjectKey: "assets/cam-1/frame-0002.jpg",
		MediaKind:      MediaKindImage,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusReady}); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress}); err != nil {
		t.Fatalf("transition to in_progress: %v", err)
	}

	submitted, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: "done"})
	if err != nil {
		t.Fatalf("transition to legacy done: %v", err)
	}
	if submitted.Status != StatusSubmitted {
		t.Fatalf("expected legacy done to resolve to %q for annotation, got %q", StatusSubmitted, submitted.Status)
	}
}

func TestServiceListTasksSupportsLegacyDoneFilter(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Closed task visible in done filter",
		Kind:      KindReview,
		Status:    StatusClosed,
	})
	if err != nil {
		t.Fatalf("create closed task: %v", err)
	}

	listed, err := svc.ListTasks(ctx, 1, ListTasksFilter{Status: "done"})
	if err != nil {
		t.Fatalf("list with done filter: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("expected legacy done filter to return closed task %d, got %+v", created.ID, listed)
	}
}

func TestServiceCreateTaskDefaultsOntologyVersion(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID:      1,
		Title:          "Ontology defaults",
		Kind:           KindAnnotation,
		AssetObjectKey: "assets/frame-100.jpg",
		MediaKind:      MediaKindImage,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if created.OntologyVersion != "v1" {
		t.Fatalf("expected default ontology_version v1, got %q", created.OntologyVersion)
	}
}

func TestServiceCreateTaskAcceptsPublishTaskKinds(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	kinds := []string{KindTrainingCandidate, KindPromotionReview}
	for _, kind := range kinds {
		created, err := svc.CreateTask(ctx, CreateTaskInput{
			ProjectID: 1,
			Title:     "publish follow-up",
			Kind:      kind,
		})
		if err != nil {
			t.Fatalf("create task with kind %q: %v", kind, err)
		}
		if created.Kind != kind {
			t.Fatalf("expected kind %q, got %q", kind, created.Kind)
		}
	}
}
