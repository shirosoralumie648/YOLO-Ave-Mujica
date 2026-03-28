package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyManifestFailsOnChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	err := VerifyFile(p, "deadbeef")
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
