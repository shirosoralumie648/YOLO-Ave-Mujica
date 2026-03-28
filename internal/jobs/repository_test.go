package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationIncludesRequiredTables(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "migrations", "000001_init.up.sql")
	b, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	ddl := string(b)

	required := []string{
		"create table datasets",
		"create table dataset_items",
		"create table dataset_snapshots",
		"create table annotations",
		"create table annotation_candidates",
		"create table jobs",
		"create table job_events",
		"create table artifacts",
	}

	for _, token := range required {
		if !strings.Contains(ddl, token) {
			t.Fatalf("migration missing token %q", token)
		}
	}
}

func TestCreateOrGetByIdempotency(t *testing.T) {
	repo := NewInMemoryRepository()

	first, created, err := repo.CreateOrGet(1, "zero-shot", "gpu", "same-key", map[string]any{"prompt": "extinguisher"})
	if err != nil {
		t.Fatalf("first create returned error: %v", err)
	}
	if !created {
		t.Fatal("expected first create to be new")
	}

	second, created, err := repo.CreateOrGet(1, "zero-shot", "gpu", "same-key", map[string]any{"prompt": "extinguisher"})
	if err != nil {
		t.Fatalf("second create returned error: %v", err)
	}
	if created {
		t.Fatal("expected second create to reuse existing job")
	}
	if first.ID != second.ID {
		t.Fatalf("expected same id, got first=%d second=%d", first.ID, second.ID)
	}
}
