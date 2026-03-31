package tasks

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestServiceCreateListAndGetTask(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	task, err := svc.CreateTask(CreateTaskInput{
		ProjectID:  1,
		SnapshotID: 7,
		Title:      "Label parking-lot batch",
		Assignee:   "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID != 1 {
		t.Fatalf("expected id 1, got %d", task.ID)
	}
	if task.Kind != KindAnnotation || task.Status != StatusQueued || task.Priority != PriorityNormal {
		t.Fatalf("expected defaults to be applied, got %+v", task)
	}

	items, err := svc.ListTasks(1, ListTasksFilter{Assignee: "annotator-1"})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Label parking-lot batch" {
		t.Fatalf("unexpected task list: %+v", items)
	}

	got, err := svc.GetTask(task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ID != task.ID || got.Assignee != "annotator-1" {
		t.Fatalf("unexpected task: %+v", got)
	}
}

func TestServiceCreateTaskRejectsNegativeSnapshotID(t *testing.T) {
	svc := NewService(NewInMemoryRepository())

	_, err := svc.CreateTask(CreateTaskInput{
		ProjectID:  1,
		SnapshotID: -1,
		Title:      "bad snapshot",
	})
	if err == nil || !strings.Contains(err.Error(), "snapshot_id") {
		t.Fatalf("expected snapshot_id validation error, got %v", err)
	}
}

func TestServiceCreateTaskRejectsInvalidEnumValues(t *testing.T) {
	tests := []struct {
		name string
		in   CreateTaskInput
		want string
	}{
		{
			name: "invalid kind",
			in: CreateTaskInput{
				ProjectID: 1,
				Title:     "bad kind",
				Kind:      "invalid",
			},
			want: "kind",
		},
		{
			name: "invalid status",
			in: CreateTaskInput{
				ProjectID: 1,
				Title:     "bad status",
				Status:    "invalid",
			},
			want: "status",
		},
		{
			name: "invalid priority",
			in: CreateTaskInput{
				ProjectID: 1,
				Title:     "bad priority",
				Priority:  "urgent",
			},
			want: "priority",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(NewInMemoryRepository())
			_, err := svc.CreateTask(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s validation error, got %v", tc.want, err)
			}
		})
	}
}

func TestInMemoryRepositoryListTasksOrdersByLastActivityAtThenID(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	first, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "first",
		Kind:      KindAnnotation,
		Status:    StatusQueued,
		Priority:  PriorityNormal,
	})
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}

	second, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "second",
		Kind:      KindAnnotation,
		Status:    StatusQueued,
		Priority:  PriorityNormal,
	})
	if err != nil {
		t.Fatalf("create second task: %v", err)
	}

	base := time.Now().UTC()
	repo.items[first.ID] = Task{
		ID:             first.ID,
		ProjectID:      first.ProjectID,
		Title:          first.Title,
		Kind:           first.Kind,
		Status:         first.Status,
		Priority:       first.Priority,
		Assignee:       first.Assignee,
		LastActivityAt: base.Add(time.Minute),
		CreatedAt:      first.CreatedAt,
		UpdatedAt:      first.UpdatedAt,
	}
	repo.items[second.ID] = Task{
		ID:             second.ID,
		ProjectID:      second.ProjectID,
		Title:          second.Title,
		Kind:           second.Kind,
		Status:         second.Status,
		Priority:       second.Priority,
		Assignee:       second.Assignee,
		LastActivityAt: base,
		CreatedAt:      second.CreatedAt,
		UpdatedAt:      second.UpdatedAt,
	}

	for i := 0; i < 20; i++ {
		items, err := repo.ListTasks(ctx, 1, ListTasksFilter{})
		if err != nil {
			t.Fatalf("list tasks: %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(items))
		}
		if items[0].ID != second.ID || items[1].ID != first.ID {
			t.Fatalf("unexpected order on iteration %d: %+v", i, items)
		}
	}
}
