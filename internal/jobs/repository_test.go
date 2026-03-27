package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationIncludesRequiredTables(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "migrations", "0001_init.sql")
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
