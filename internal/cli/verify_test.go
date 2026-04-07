package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifyManifestEntriesUsesMultipleWorkers(t *testing.T) {
	dir := t.TempDir()
	bodies := map[string][]byte{
		"a.txt": []byte("aaaaa"),
		"b.txt": []byte("bbbbb"),
		"c.txt": []byte("ccccc"),
	}
	entries := make([]ManifestEntry, 0, len(bodies))
	for path, body := range bodies {
		if err := os.WriteFile(filepath.Join(dir, path), body, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
		entries = append(entries, ManifestEntry{
			Path:     path,
			Size:     int64(len(body)),
			Checksum: "sha256:" + checksumHex(body),
		})
	}

	started := make(chan struct{}, len(entries))
	release := make(chan struct{})
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	type outcome struct {
		results []VerifyFileResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		results, err := verifyManifestEntries(context.Background(), dir, entries, len(entries), func(path string, entry ManifestEntry) (VerifyFileResult, error) {
			current := inFlight.Add(1)
			for {
				recorded := maxInFlight.Load()
				if current <= recorded || maxInFlight.CompareAndSwap(recorded, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			result, err := VerifyManifestEntry(path, entry)
			inFlight.Add(-1)
			return result, err
		})
		done <- outcome{results: results, err: err}
	}()

	timeout := time.After(2 * time.Second)
	for range entries {
		select {
		case <-started:
		case <-timeout:
			t.Fatal("expected multiple verification workers to start before release")
		}
	}
	close(release)

	out := <-done
	if out.err != nil {
		t.Fatalf("verify manifest entries: %v", out.err)
	}
	if len(out.results) != len(entries) {
		t.Fatalf("expected %d verification results, got %d", len(entries), len(out.results))
	}
	if maxInFlight.Load() < 2 {
		t.Fatalf("expected verification to use multiple workers, max in flight=%d", maxInFlight.Load())
	}
}
