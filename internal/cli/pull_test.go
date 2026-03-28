package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fakeSource struct {
	pkg PulledArtifact
	err error
}

func (f fakeSource) FetchArtifact(format, version string) (PulledArtifact, error) {
	return f.pkg, f.err
}

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

func TestPullWritesVerifyReport(t *testing.T) {
	dir := t.TempDir()
	client := NewPullClientWithSource(dir, fakeSource{
		pkg: PulledArtifact{
			ArtifactID: 1,
			Version:    "v1",
			Entries: []ArtifactEntry{
				{
					Path:     "labels/0001.txt",
					Body:     []byte("0 0.5 0.5 0.2 0.2\n"),
					Checksum: "fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549",
				},
			},
		},
	})

	err := client.Pull(PullOptions{Format: "yolo", Version: "v1", AllowPartial: false, OutputDir: dir})
	if err != nil {
		t.Fatalf("pull returned error: %v", err)
	}

	reportPath := filepath.Join(dir, "verify-report.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("missing verify-report.json: %v", err)
	}

	b, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read verify report: %v", err)
	}
	var report VerifyReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("unmarshal verify report: %v", err)
	}
	if report.Snapshot != "v1" {
		t.Fatalf("expected snapshot v1, got %s", report.Snapshot)
	}
	if report.FailedFiles != 0 {
		t.Fatalf("expected zero failed files, got %d", report.FailedFiles)
	}
	if report.EnvironmentContext.OS == "" || report.EnvironmentContext.StorageDriver == "" {
		t.Fatalf("expected environment context, got %+v", report.EnvironmentContext)
	}

	pulledFile := filepath.Join(dir, "pulled-v1", "labels", "0001.txt")
	if _, err := os.Stat(pulledFile); err != nil {
		t.Fatalf("expected pulled file from source, got %v", err)
	}
}

func TestPullAllowPartialControlsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	client := NewPullClientWithSource(dir, fakeSource{
		pkg: PulledArtifact{
			ArtifactID: 2,
			Version:    "v2",
			Entries: []ArtifactEntry{
				{
					Path:     "labels/0001.txt",
					Body:     []byte("0 0.5 0.5 0.2 0.2\n"),
					Checksum: "deadbeef",
				},
			},
		},
	})

	err := client.Pull(PullOptions{
		Format:       "yolo",
		Version:      "v2",
		AllowPartial: false,
		OutputDir:    dir,
	})
	if err == nil {
		t.Fatal("expected checksum mismatch when allow-partial is false")
	}

	err = client.Pull(PullOptions{
		Format:       "yolo",
		Version:      "v2",
		AllowPartial: true,
		OutputDir:    dir,
	})
	if err != nil {
		t.Fatalf("expected allow-partial flow to succeed, got %v", err)
	}
}

func TestPullWithoutSourceFails(t *testing.T) {
	dir := t.TempDir()
	client := NewPullClient(dir)

	err := client.Pull(PullOptions{Format: "yolo", Version: "v1", OutputDir: dir})
	if err == nil {
		t.Fatal("expected pull without source to fail")
	}
}
