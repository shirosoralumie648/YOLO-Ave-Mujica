package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	resolved    ResolvedArtifact
	resolveErr  error
	downloadErr error
	download    func(context.Context, ResolvedArtifact, string) error
}

func (f fakeSource) ResolveArtifact(_ string, _ string, _ string) (ResolvedArtifact, error) {
	return f.resolved, f.resolveErr
}

func (f fakeSource) DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error {
	if f.download != nil {
		return f.download(ctx, artifact, tempPath)
	}
	return f.downloadErr
}

type recordingSource struct {
	dataset     string
	format      string
	version     string
	resolved    ResolvedArtifact
	archivePath string
}

func (r *recordingSource) ResolveArtifact(dataset, format, version string) (ResolvedArtifact, error) {
	r.dataset = dataset
	r.format = format
	r.version = version
	return r.resolved, nil
}

func (r *recordingSource) DownloadArchive(_ context.Context, _ ResolvedArtifact, tempPath string) error {
	body, err := os.ReadFile(r.archivePath)
	if err != nil {
		return err
	}
	return os.WriteFile(tempPath, body, 0o644)
}

type pollingSource struct {
	attempts    int
	archivePath string
	resolved    ResolvedArtifact
}

func (p *pollingSource) ResolveArtifact(_ string, _ string, _ string) (ResolvedArtifact, error) {
	p.attempts++
	if p.attempts < 3 {
		return ResolvedArtifact{}, ErrArtifactUnavailable
	}
	return p.resolved, nil
}

func (p *pollingSource) DownloadArchive(_ context.Context, _ ResolvedArtifact, tempPath string) error {
	body, err := os.ReadFile(p.archivePath)
	if err != nil {
		return err
	}
	return os.WriteFile(tempPath, body, 0o644)
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

func TestVerifyManifestEntryRejectsSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	body := []byte("hello")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	_, err := VerifyManifestEntry(path, ManifestEntry{
		Path:     "sample.txt",
		Size:     int64(len(body) + 1),
		Checksum: "sha256:" + checksumHex(body),
	})
	if err == nil {
		t.Fatal("expected size mismatch to be rejected")
	}
	if !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("expected size mismatch error, got %v", err)
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

	err := client.Pull(PullOptions{
		Dataset:       "smoke-dataset",
		Format:        "yolo",
		Version:       "v1",
		AllowPartial:  false,
		OutputDir:     dir,
		VerifyWorkers: 1,
	})
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
	if report.VerifiedFiles != 1 {
		t.Fatalf("expected one verified file, got %d", report.VerifiedFiles)
	}
	if report.VerificationWorkers != 1 {
		t.Fatalf("expected one verification worker, got %d", report.VerificationWorkers)
	}
	if report.EnvironmentContext.OS == "" || report.EnvironmentContext.StorageDriver == "" {
		t.Fatalf("expected environment context, got %+v", report.EnvironmentContext)
	}
	if len(report.Files) != 1 {
		t.Fatalf("expected one file result, got %+v", report.Files)
	}
	if report.Files[0].Path != "labels/0001.txt" || report.Files[0].Status != VerifyStatusPassed {
		t.Fatalf("unexpected file verification result: %+v", report.Files[0])
	}
	if report.Files[0].Size == 0 {
		t.Fatalf("expected file size in report, got %+v", report.Files[0])
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
		Dataset:      "smoke-dataset",
		Format:       "yolo",
		Version:      "v2",
		AllowPartial: false,
		OutputDir:    dir,
	})
	if err == nil {
		t.Fatal("expected checksum mismatch when allow-partial is false")
	}

	err = client.Pull(PullOptions{
		Dataset:       "smoke-dataset",
		Format:        "yolo",
		Version:       "v2",
		AllowPartial:  true,
		OutputDir:     dir,
		VerifyWorkers: 2,
	})
	if err != nil {
		t.Fatalf("expected allow-partial flow to succeed, got %v", err)
	}

	reportPath := filepath.Join(dir, "verify-report.json")
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read verify report: %v", err)
	}
	var report VerifyReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("unmarshal verify report: %v", err)
	}
	if report.FailedFiles != 1 || report.VerifiedFiles != 0 {
		t.Fatalf("expected one failed file and zero verified files, got %+v", report)
	}
	if report.VerificationWorkers != 2 {
		t.Fatalf("expected verification worker count 2, got %d", report.VerificationWorkers)
	}
	if len(report.Files) != 1 {
		t.Fatalf("expected one file verification result, got %+v", report.Files)
	}
	if report.Files[0].Status != VerifyStatusFailed {
		t.Fatalf("expected failed file status, got %+v", report.Files[0])
	}
	if !strings.Contains(report.Files[0].Error, "checksum mismatch") {
		t.Fatalf("expected checksum mismatch detail, got %+v", report.Files[0])
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
	if _, err := os.Stat(filepath.Join(dir, "pull-v1.tar.gz.part")); !os.IsNotExist(err) {
		t.Fatalf("expected temporary archive cleanup, got err=%v", err)
	}
}

func TestPullWithoutSourceFails(t *testing.T) {
	dir := t.TempDir()
	client := NewPullClient(dir)

	err := client.Pull(PullOptions{Dataset: "smoke-dataset", Format: "yolo", Version: "v1", OutputDir: dir})
	if err == nil {
		t.Fatal("expected pull without source to fail")
	}
}

func TestHelpTextShowsDatasetVersionFormatAndAllowPartial(t *testing.T) {
	text := helpText()
	for _, token := range []string{"pull", "--dataset", "--version", "--format", "--allow-partial", "--wait-timeout", "--poll-interval", "--verify-workers"} {
		if !strings.Contains(text, token) {
			t.Fatalf("expected help text to contain %s, got:\n%s", token, text)
		}
	}
}

func TestPullPassesDatasetToSource(t *testing.T) {
	dir := t.TempDir()
	archivePath := writeTestArchive(t, testArchive{
		Version: "v3",
		Files: map[string][]byte{
			"train/labels/a.txt": []byte("0 0.5 0.5 0.2 0.2\n"),
		},
	})
	source := &recordingSource{
		resolved:    ResolvedArtifact{ArtifactID: 7, Version: "v3", DownloadURL: "http://example.test/v1/artifacts/7/download"},
		archivePath: archivePath,
	}
	client := NewPullClientWithSource(dir, source)

	err := client.Pull(PullOptions{
		Dataset:   "smoke-dataset",
		Format:    "yolo",
		Version:   "v3",
		OutputDir: dir,
	})
	if err != nil {
		t.Fatalf("pull returned error: %v", err)
	}
	if source.dataset != "smoke-dataset" || source.format != "yolo" || source.version != "v3" {
		t.Fatalf("expected dataset/format/version to be forwarded, got dataset=%q format=%q version=%q", source.dataset, source.format, source.version)
	}
}

func TestPullPollsUntilArtifactBecomesAvailable(t *testing.T) {
	dir := t.TempDir()
	archivePath := writeTestArchive(t, testArchive{
		Version: "v4",
		Files: map[string][]byte{
			"train/labels/a.txt": []byte("0 0.5 0.5 0.2 0.2\n"),
		},
	})
	source := &pollingSource{
		archivePath: archivePath,
		resolved:    ResolvedArtifact{ArtifactID: 8, Version: "v4", DownloadURL: "http://example.test/v1/artifacts/8/download"},
	}
	client := NewPullClientWithSource(dir, source)

	err := client.Pull(PullOptions{
		Dataset:        "smoke-dataset",
		Format:         "yolo",
		Version:        "v4",
		OutputDir:      dir,
		ResolveTimeout: time.Second,
		PollInterval:   time.Millisecond,
		VerifyWorkers:  1,
	})
	if err != nil {
		t.Fatalf("pull returned error: %v", err)
	}
	if source.attempts != 3 {
		t.Fatalf("expected three resolve attempts, got %d", source.attempts)
	}
}

func TestAPIArtifactSourceResolveArtifactIncludesDatasetQuery(t *testing.T) {
	source := NewAPIArtifactSource("http://artifact.test")
	source.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/artifacts/resolve" {
			return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
		}
		if req.URL.Query().Get("dataset") != "smoke-dataset" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "missing dataset query"}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id":           5,
			"version":      "v1",
			"download_url": "/v1/artifacts/5/download",
		}), nil
	})}

	resolved, err := source.ResolveArtifact("smoke-dataset", "yolo", "v1")
	if err != nil {
		t.Fatalf("resolve artifact: %v", err)
	}
	if resolved.ArtifactID != 5 || resolved.Version != "v1" {
		t.Fatalf("unexpected resolved artifact: %+v", resolved)
	}
	if resolved.DownloadURL != "http://artifact.test/v1/artifacts/5/download" {
		t.Fatalf("unexpected absolute download url: %s", resolved.DownloadURL)
	}
}

func TestAPIArtifactSourceResolveArtifactTreatsNotFoundAsUnavailable(t *testing.T) {
	source := NewAPIArtifactSource("http://artifact.test")
	source.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
	})}

	_, err := source.ResolveArtifact("smoke-dataset", "yolo", "v1")
	if !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
