package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
