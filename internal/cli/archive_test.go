package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type testArchive struct {
	Version           string
	Files             map[string][]byte
	ManifestChecksums map[string]string
}

func TestDownloadArchiveToTempResumesFromExistingPartial(t *testing.T) {
	archivePath := writeTestArchive(t, testArchive{
		Version: "v1",
		Files: map[string][]byte{
			"train/images/a.jpg": []byte("image-a"),
		},
	})
	archiveBody, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/artifacts/7/download" {
				return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
			}
			return archiveResponse(req, archiveBody), nil
		}),
	}

	tempPath := filepath.Join(t.TempDir(), "resume.tar.gz.part")
	if err := os.WriteFile(tempPath, archiveBody[:len(archiveBody)/2], 0o644); err != nil {
		t.Fatalf("seed partial archive: %v", err)
	}

	if err := downloadArchiveToTemp(context.Background(), client, "http://example.test/v1/artifacts/7/download", tempPath); err != nil {
		t.Fatalf("resume download: %v", err)
	}

	got, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("read resumed archive: %v", err)
	}
	if !bytes.Equal(got, archiveBody) {
		t.Fatal("expected resumed download to match archive body")
	}
}

func newArtifactDownloadSource(t *testing.T) *APIArtifactSource {
	t.Helper()
	archivePath := writeTestArchive(t, testArchive{
		Version: "v1",
		Files: map[string][]byte{
			"train/images/a.jpg": []byte("image-a"),
			"train/labels/a.txt": []byte("0 0.5 0.5 0.2 0.2\n"),
			"data.yaml":          []byte("train: ./train/images\nval: ./train/images\nnames:\n  - person\n"),
		},
	})
	archiveBody, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive fixture: %v", err)
	}

	return &APIArtifactSource{
		BaseURL: "http://example.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/v1/artifacts/resolve":
					return jsonResponse(http.StatusOK, map[string]any{
						"id":           7,
						"version":      "v1",
						"download_url": "http://example.test/v1/artifacts/7/download",
					}), nil
				case "/v1/artifacts/7/download":
					return archiveResponse(req, archiveBody), nil
				default:
					return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
				}
			}),
		},
	}
}

func copyArchiveDownloader(t *testing.T, archivePath string) func(context.Context, ResolvedArtifact, string) error {
	t.Helper()
	return func(_ context.Context, _ ResolvedArtifact, tempPath string) error {
		body, err := os.ReadFile(archivePath)
		if err != nil {
			return err
		}
		return os.WriteFile(tempPath, body, 0o644)
	}
}

func archiveResponse(req *http.Request, archiveBody []byte) *http.Response {
	body := archiveBody
	status := http.StatusOK
	header := make(http.Header)
	header.Set("Content-Type", "application/gzip")

	if rangeHeader := req.Header.Get("Range"); rangeHeader != "" {
		const prefix = "bytes="
		if strings.HasPrefix(rangeHeader, prefix) {
			start, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rangeHeader, prefix), "-"))
			if err == nil && start >= 0 && start < len(archiveBody) {
				status = http.StatusPartialContent
				body = archiveBody[start:]
				header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(archiveBody)-1, len(archiveBody)))
			}
		}
	}
	header.Set("Content-Length", strconv.Itoa(len(body)))

	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func jsonResponse(status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func writeTestArchive(t *testing.T, archive testArchive) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "package.yolo.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	files := make(map[string][]byte, len(archive.Files)+1)
	for path, body := range archive.Files {
		files[path] = body
	}

	manifest := map[string]any{
		"version": archive.Version,
		"entries": buildManifestEntries(archive.Files, archive.ManifestChecksums),
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	files["manifest.json"] = manifestBody

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, filePath := range paths {
		body := files[filePath]
		header := &tar.Header{
			Name:    filePath,
			Mode:    0o644,
			ModTime: testModTime(),
			Size:    int64(len(body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	return archivePath
}

func buildManifestEntries(files map[string][]byte, overrides map[string]string) []map[string]any {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	entries := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		checksum := "sha256:" + checksumHex(files[path])
		if override, ok := overrides[path]; ok {
			checksum = override
		}
		entries = append(entries, map[string]any{
			"path":     path,
			"size":     len(files[path]),
			"checksum": checksum,
		})
	}
	return entries
}

func checksumHex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func testModTime() time.Time {
	return time.Unix(0, 0).UTC()
}
