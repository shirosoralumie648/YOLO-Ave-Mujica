package artifacts

import (
	"bytes"
	"testing"
)

func TestApplyLabelMap(t *testing.T) {
	labels := []string{"pedestrian", "car"}
	mapped := ApplyLabelMap(labels, map[string]string{"pedestrian": "person"})
	if mapped[0] != "person" {
		t.Fatalf("expected person, got %s", mapped[0])
	}
}

func TestBuildManifestIncludesChecksums(t *testing.T) {
	entries := []ManifestEntry{{Path: "labels/0001.txt", Checksum: "abc123"}}
	b, err := BuildManifest("v1.2", entries)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if !bytes.Contains(b, []byte(`"checksum": "sha256:abc123"`)) {
		t.Fatalf("manifest missing labeled checksum: %s", b)
	}
}
