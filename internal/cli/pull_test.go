package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSource struct {
	pkg PulledArtifact
	err error
}

func (f fakeSource) FetchArtifact(dataset, format, version string) (PulledArtifact, error) {
	return f.pkg, f.err
}

type recordingSource struct {
	dataset string
	format  string
	version string
	pkg     PulledArtifact
}

func (r *recordingSource) FetchArtifact(dataset, format, version string) (PulledArtifact, error) {
	r.dataset = dataset
	r.format = format
	r.version = version
	return r.pkg, nil
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

	err := client.Pull(PullOptions{Dataset: "smoke-dataset", Format: "yolo", Version: "v1", AllowPartial: false, OutputDir: dir})
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
		Dataset:      "smoke-dataset",
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

	err := client.Pull(PullOptions{Dataset: "smoke-dataset", Format: "yolo", Version: "v1", OutputDir: dir})
	if err == nil {
		t.Fatal("expected pull without source to fail")
	}
}

func TestHelpTextShowsDatasetVersionFormatAndAllowPartial(t *testing.T) {
	text := helpText()
	for _, token := range []string{"pull", "--dataset", "--version", "--format", "--allow-partial"} {
		if !strings.Contains(text, token) {
			t.Fatalf("expected help text to contain %s, got:\n%s", token, text)
		}
	}
}

func TestPullPassesDatasetToSource(t *testing.T) {
	dir := t.TempDir()
	source := &recordingSource{
		pkg: PulledArtifact{
			ArtifactID: 7,
			Version:    "v3",
			Entries: []ArtifactEntry{
				{
					Path:     "labels/0001.txt",
					Body:     []byte("0 0.5 0.5 0.2 0.2\n"),
					Checksum: "fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549",
				},
			},
		},
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

func TestAPIArtifactSourceFetchesBundleEntries(t *testing.T) {
	source := NewAPIArtifactSource("http://artifact.test")
	source.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/artifacts/resolve":
			if req.URL.Query().Get("dataset") != "smoke-dataset" {
				return jsonResponse(http.StatusBadRequest, `{"error":"missing dataset query"}`), nil
			}
			return jsonResponse(http.StatusOK, `{"id":5,"version":"v1"}`), nil
		case "/v1/artifacts/5":
			return jsonResponse(http.StatusOK, `{"id":5,"version":"v1","entries":[{"path":"labels/0001.txt","body":"MCAwLjUgMC41IDAuMiAwLjIK","checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"}]}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})}

	pkg, err := source.FetchArtifact("smoke-dataset", "yolo", "v1")
	if err != nil {
		t.Fatalf("fetch artifact: %v", err)
	}
	if len(pkg.Entries) == 0 {
		t.Fatalf("expected bundle entries, got %+v", pkg)
	}
	if pkg.Entries[0].Path == "" || len(pkg.Entries[0].Body) == 0 {
		t.Fatalf("expected populated bundle entry, got %+v", pkg.Entries[0])
	}
}

func TestAPIArtifactSourceDownloadsFromPresignedArtifactURL(t *testing.T) {
	source := NewAPIArtifactSource("http://artifact.test")
	source.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/artifacts/resolve":
			return jsonResponse(http.StatusOK, `{"id":9,"version":"v2"}`), nil
		case "/v1/artifacts/9":
			return jsonResponse(http.StatusOK, `{"id":9,"version":"v2","status":"ready","entries":[]}`), nil
		case "/v1/artifacts/9/presign":
			return jsonResponse(http.StatusOK, `{"url":"http://artifact.test/download/9"}`), nil
		case "/download/9":
			return jsonResponse(http.StatusOK, `{"artifact_id":9,"version":"v2","entries":[{"path":"labels/0001.txt","body":"MCAwLjUgMC41IDAuMiAwLjIK","checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"}]}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})}

	pkg, err := source.FetchArtifact("smoke-dataset", "yolo", "v2")
	if err != nil {
		t.Fatalf("fetch artifact: %v", err)
	}
	if pkg.ArtifactID != 9 || pkg.Version != "v2" {
		t.Fatalf("unexpected pulled package identity: %+v", pkg)
	}
	if len(pkg.Entries) != 1 || pkg.Entries[0].Path != "labels/0001.txt" {
		t.Fatalf("expected downloaded package entries, got %+v", pkg)
	}
}

func TestAPIArtifactSourceWaitsForReadyArtifactBeforePresign(t *testing.T) {
	source := NewAPIArtifactSource("http://artifact.test")
	var artifactFetches int
	source.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/artifacts/resolve":
			return jsonResponse(http.StatusOK, `{"id":11,"version":"v3"}`), nil
		case "/v1/artifacts/11":
			artifactFetches++
			if artifactFetches == 1 {
				return jsonResponse(http.StatusOK, `{"id":11,"version":"v3","status":"queued","entries":[]}`), nil
			}
			return jsonResponse(http.StatusOK, `{"id":11,"version":"v3","status":"ready","entries":[]}`), nil
		case "/v1/artifacts/11/presign":
			return jsonResponse(http.StatusOK, `{"url":"http://artifact.test/download/11"}`), nil
		case "/download/11":
			return jsonResponse(http.StatusOK, `{"artifact_id":11,"version":"v3","entries":[{"path":"labels/0001.txt","body":"MCAwLjUgMC41IDAuMiAwLjIK","checksum":"fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549"}]}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})}

	pkg, err := source.FetchArtifact("smoke-dataset", "yolo", "v3")
	if err != nil {
		t.Fatalf("fetch artifact: %v", err)
	}
	if artifactFetches < 2 {
		t.Fatalf("expected source to poll artifact readiness, got %d fetches", artifactFetches)
	}
	if pkg.ArtifactID != 11 || len(pkg.Entries) != 1 {
		t.Fatalf("expected downloaded package after readiness poll, got %+v", pkg)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
