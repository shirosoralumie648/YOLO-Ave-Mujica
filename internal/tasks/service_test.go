package tasks

import (
	"testing"
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
