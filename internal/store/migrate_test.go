package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type migrationDirectionFiles map[string][]string

func migrationFilesByVersion(root string) (map[string]migrationDirectionFiles, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	versions := make(map[string]migrationDirectionFiles)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) < 2 {
			continue
		}
		version := parts[0]
		direction := ""
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			direction = "up"
		case strings.HasSuffix(name, ".down.sql"):
			direction = "down"
		default:
			continue
		}

		if _, ok := versions[version]; !ok {
			versions[version] = migrationDirectionFiles{
				"up":   []string{},
				"down": []string{},
			}
		}
		versions[version][direction] = append(versions[version][direction], filepath.Join(root, name))
	}
	return versions, nil
}

func mustReadMigration(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	return string(body)
}

func TestBaselineMigrationFilesExist(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	up := filepath.Join(root, "000001_init.up.sql")
	down := filepath.Join(root, "000001_init.down.sql")

	for _, path := range []string{up, down} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected migration file %s: %v", path, err)
		}
	}

	body, err := os.ReadFile(up)
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if !strings.Contains(string(body), "create table jobs") {
		t.Fatalf("expected jobs table in baseline migration")
	}
}

func TestMigrationVersionsArePairedAndUnique(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	versions, err := migrationFilesByVersion(root)
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one migration version")
	}

	for version, files := range versions {
		if len(files["up"]) != 1 {
			t.Fatalf("expected exactly one up migration for version %s, got %v", version, files["up"])
		}
		if len(files["down"]) != 1 {
			t.Fatalf("expected exactly one down migration for version %s, got %v", version, files["down"])
		}
	}
}

func TestTasksMigrationUpgradePathIsConsistent(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	kernelUp := mustReadMigration(t, filepath.Join(root, "000003_task_overview_kernel.up.sql"))
	workspaceUp := mustReadMigration(t, filepath.Join(root, "000005_annotation_workspace_foundation.up.sql"))
	workspaceDown := mustReadMigration(t, filepath.Join(root, "000005_annotation_workspace_foundation.down.sql"))

	if !strings.Contains(kernelUp, "create table tasks") {
		t.Fatalf("expected 000003 to create tasks table, got:\n%s", kernelUp)
	}
	if !strings.Contains(kernelUp, "status in ('queued', 'ready', 'in_progress', 'blocked', 'done')") {
		t.Fatalf("expected 000003 to define pre-workspace task statuses, got:\n%s", kernelUp)
	}
	if !strings.Contains(workspaceUp, "set status = 'closed'") {
		t.Fatalf("expected 000005 up migration to translate done -> closed, got:\n%s", workspaceUp)
	}
	for _, status := range []string{"submitted", "reviewing", "rework_required", "accepted", "published", "closed"} {
		if !strings.Contains(workspaceUp, "'"+status+"'") {
			t.Fatalf("expected 000005 up migration to include status %q, got:\n%s", status, workspaceUp)
		}
	}
	for _, kind := range []string{"training_candidate", "promotion_review"} {
		if !strings.Contains(workspaceUp, "'"+kind+"'") {
			t.Fatalf("expected 000005 up migration to include task kind %q, got:\n%s", kind, workspaceUp)
		}
	}
	if !strings.Contains(workspaceDown, "set status = 'done'") {
		t.Fatalf("expected 000005 down migration to translate extended statuses back to done, got:\n%s", workspaceDown)
	}
	if !strings.Contains(workspaceDown, "status in ('queued', 'ready', 'in_progress', 'blocked', 'done')") {
		t.Fatalf("expected 000005 down migration to restore pre-workspace task statuses, got:\n%s", workspaceDown)
	}
}
