package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemStoragePromotesArchiveAtomically(t *testing.T) {
	root := t.TempDir()
	storage := NewFilesystemStorage(root)
	srcDir := t.TempDir()

	packageDir := filepath.Join(srcDir, "package")
	if err := os.MkdirAll(filepath.Join(packageDir, "train", "labels"), 0o755); err != nil {
		t.Fatalf("mkdir package dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "manifest.json"), []byte(`{"version":"v1"}`), 0o644); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}

	archivePath := filepath.Join(srcDir, "package.yolo.tar.gz")
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	loc, err := storage.StoreBuild(context.Background(), StoreRequest{
		Version:      "v1",
		ArchivePath:  archivePath,
		ManifestPath: filepath.Join(packageDir, "manifest.json"),
		PackageDir:   packageDir,
	})
	if err != nil {
		t.Fatalf("store build: %v", err)
	}
	if strings.Contains(loc.ArchivePath, ".tmp") {
		t.Fatalf("expected final archive path, got %s", loc.ArchivePath)
	}
	if _, err := os.Stat(filepath.Join(root, "v1", "package.yolo.tar.gz")); err != nil {
		t.Fatalf("missing promoted archive: %v", err)
	}
}
