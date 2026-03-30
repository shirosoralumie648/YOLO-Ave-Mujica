package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeSource struct {
	resolved    ResolvedArtifact
	resolveErr  error
	downloadErr error
	download    func(context.Context, ResolvedArtifact, string) error
}

func (f fakeSource) ResolveArtifact(format, version string) (ResolvedArtifact, error) {
	return f.resolved, f.resolveErr
}

func (f fakeSource) DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error {
	if f.download != nil {
		return f.download(ctx, artifact, tempPath)
	}
	return f.downloadErr
}

func TestVerifyManifestFailsOnChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	err := VerifyFile(p, "sha256:deadbeef")
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestVerifyFileRejectsNonSHA256Checksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	if err := VerifyFile(path, "md5:deadbeef"); err == nil {
		t.Fatal("expected non-sha256 checksum to be rejected")
	}
}

func TestPullWritesVerifyReport(t *testing.T) {
	dir := t.TempDir()
	archivePath := writeTestArchive(t, testArchive{
		Version: "v1",
		Files: map[string][]byte{
			"labels/0001.txt": []byte("0 0.5 0.5 0.2 0.2\n"),
		},
	})
	client := NewPullClientWithSource(dir, fakeSource{
		resolved: ResolvedArtifact{ArtifactID: 1, Version: "v1", DownloadURL: "http://example.test/v1/artifacts/1/download"},
		download: copyArchiveDownloader(t, archivePath),
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
	archivePath := writeTestArchive(t, testArchive{
		Version: "v2",
		Files: map[string][]byte{
			"labels/0001.txt": []byte("0 0.5 0.5 0.2 0.2\n"),
		},
		ManifestChecksums: map[string]string{
			"labels/0001.txt": "sha256:deadbeef",
		},
	})
	client := NewPullClientWithSource(dir, fakeSource{
		resolved: ResolvedArtifact{ArtifactID: 2, Version: "v2", DownloadURL: "http://example.test/v1/artifacts/2/download"},
		download: copyArchiveDownloader(t, archivePath),
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

func TestPullDownloadsArchiveExtractsManifestAndCleansTempArchive(t *testing.T) {
	dir := t.TempDir()
	client := NewPullClientWithSource(dir, newArtifactDownloadSource(t))

	err := client.Pull(PullOptions{Format: "yolo", Version: "v1", OutputDir: dir})
	if err != nil {
		t.Fatalf("pull returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "pulled-v1", "train", "images", "a.jpg")); err != nil {
		t.Fatalf("missing extracted image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pull-v1.tar.gz.part")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temporary archive cleanup, got err=%v", err)
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
