package tasks

import (
	"strings"
	"testing"
)

func TestServiceCreateListAndGetTask(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(repo)
	datasetID := int64(3)
	snapshotID := int64(7)

	task, err := svc.CreateTask(CreateTaskInput{
		ProjectID:   1,
		DatasetID:   &datasetID,
		SnapshotID:  &snapshotID,
		Title:       "  Label parking-lot batch  ",
		Description: "Triage the imported batch before review.",
		Assignee:    "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID != 1 {
		t.Fatalf("expected id 1, got %d", task.ID)
	}
	if task.Title != "Label parking-lot batch" {
		t.Fatalf("expected trimmed title, got %+v", task)
	}
	if task.Status != StatusReady || task.Priority != PriorityNormal {
		t.Fatalf("expected defaults to be applied, got %+v", task)
	}
	if task.DatasetID == nil || *task.DatasetID != datasetID {
		t.Fatalf("expected dataset id %d, got %+v", datasetID, task.DatasetID)
	}
	if task.SnapshotID == nil || *task.SnapshotID != snapshotID {
		t.Fatalf("expected snapshot id %d, got %+v", snapshotID, task.SnapshotID)
	}

	items, err := svc.ListProjectTasks(1)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Label parking-lot batch" {
		t.Fatalf("unexpected task list: %+v", items)
	}

	got, ok, err := svc.GetTask(task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !ok {
		t.Fatalf("expected task to exist")
	}
	if got.ID != task.ID || got.Assignee != "annotator-1" {
		t.Fatalf("unexpected task: %+v", got)
	}
}

func TestServiceCreateTaskRejectsInvalidIdentifiers(t *testing.T) {
	svc := NewServiceWithRepository(NewInMemoryRepository())
	invalidDatasetID := int64(0)
	invalidSnapshotID := int64(-1)

	tests := []struct {
		name string
		in   CreateTaskInput
		want string
	}{
		{
			name: "invalid dataset id",
			in: CreateTaskInput{
				ProjectID: 1,
				DatasetID: &invalidDatasetID,
				Title:     "bad dataset",
			},
			want: "dataset_id",
		},
		{
			name: "invalid snapshot id",
			in: CreateTaskInput{
				ProjectID:  1,
				SnapshotID: &invalidSnapshotID,
				Title:      "bad snapshot",
			},
			want: "snapshot_id",
		},
		{
			name: "missing title after trim",
			in: CreateTaskInput{
				ProjectID: 1,
				Title:     "   ",
			},
			want: "title",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateTask(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s validation error, got %v", tc.want, err)
			}
		})
	}
}

func TestServiceCreateTaskRejectsInvalidEnumValues(t *testing.T) {
	tests := []struct {
		name string
		in   CreateTaskInput
		want string
	}{
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
			svc := NewServiceWithRepository(NewInMemoryRepository())
			_, err := svc.CreateTask(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s validation error, got %v", tc.want, err)
			}
		})
	}
}
