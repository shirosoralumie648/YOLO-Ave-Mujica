package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type EnvironmentContext struct {
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CLIVersion    string `json:"cli_version"`
	StorageDriver string `json:"storage_driver"`
}

type VerifyReport struct {
	ArtifactID          int64              `json:"artifact_id"`
	Snapshot            string             `json:"snapshot"`
	TotalFiles          int                `json:"total_files"`
	VerifiedFiles       int                `json:"verified_files"`
	FailedFiles         int                `json:"failed_files"`
	VerificationWorkers int                `json:"verification_workers"`
	Files               []VerifyFileResult `json:"files"`
	VerifiedAt          string             `json:"verified_at"`
	EnvironmentContext  EnvironmentContext `json:"environment_context"`
}

type VerifyFileResult struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

const (
	VerifyStatusPassed = "passed"
	VerifyStatusFailed = "failed"
)

func VerifyFile(path string, expectedSHA256 string) error {
	parts := strings.SplitN(expectedSHA256, ":", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm %q", expectedSHA256)
	}
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != parts[1] {
		return fmt.Errorf("checksum mismatch: got sha256:%s expected %s", got, expectedSHA256)
	}
	return nil
}

func VerifyManifestEntry(path string, entry ManifestEntry) (VerifyFileResult, error) {
	result := VerifyFileResult{
		Path:     entry.Path,
		Size:     entry.Size,
		Checksum: entry.Checksum,
		Status:   VerifyStatusPassed,
	}

	if entry.Size > 0 {
		info, err := os.Stat(path)
		if err != nil {
			result.Status = VerifyStatusFailed
			result.Error = err.Error()
			return result, err
		}
		if info.Size() != entry.Size {
			err := fmt.Errorf("size mismatch: got %d expected %d", info.Size(), entry.Size)
			result.Status = VerifyStatusFailed
			result.Error = err.Error()
			return result, err
		}
	}

	if err := VerifyFile(path, entry.Checksum); err != nil {
		result.Status = VerifyStatusFailed
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

type verifyPathFunc func(path string, entry ManifestEntry) (VerifyFileResult, error)

func verifyManifestEntries(ctx context.Context, rootDir string, entries []ManifestEntry, workers int, verify verifyPathFunc) ([]VerifyFileResult, error) {
	if verify == nil {
		verify = VerifyManifestEntry
	}
	if workers <= 0 {
		workers = 1
	}
	type verifyJob struct {
		index int
		entry ManifestEntry
	}
	type verifyOutcome struct {
		index  int
		result VerifyFileResult
		err    error
	}

	jobs := make(chan verifyJob)
	outcomes := make(chan verifyOutcome, len(entries))
	workerCount := workers
	if workerCount > len(entries) && len(entries) > 0 {
		workerCount = len(entries)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := ctx.Err(); err != nil {
					outcomes <- verifyOutcome{
						index: job.index,
						result: VerifyFileResult{
							Path:     job.entry.Path,
							Size:     job.entry.Size,
							Checksum: job.entry.Checksum,
							Status:   VerifyStatusFailed,
							Error:    err.Error(),
						},
						err: err,
					}
					continue
				}

				targetPath := filepath.Join(rootDir, filepath.FromSlash(job.entry.Path))
				result, err := verify(targetPath, job.entry)
				outcomes <- verifyOutcome{index: job.index, result: result, err: err}
			}
		}()
	}

	go func() {
		for index, entry := range entries {
			jobs <- verifyJob{index: index, entry: entry}
		}
		close(jobs)
		wg.Wait()
		close(outcomes)
	}()

	results := make([]VerifyFileResult, len(entries))
	var firstErr error
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		if outcome.err != nil && firstErr == nil {
			firstErr = outcome.err
		}
	}
	return results, firstErr
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeVerifyReport(path string, report VerifyReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func buildEnvironmentContext(source ArtifactSource) EnvironmentContext {
	driver := "artifact-source"
	if _, ok := source.(*APIArtifactSource); ok {
		driver = "http"
	}
	return EnvironmentContext{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		CLIVersion:    "dev",
		StorageDriver: driver,
	}
}
