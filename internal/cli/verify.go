package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
)

type EnvironmentContext struct {
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CLIVersion    string `json:"cli_version"`
	StorageDriver string `json:"storage_driver"`
}

type VerifyReport struct {
	ArtifactID         int64              `json:"artifact_id"`
	Snapshot           string             `json:"snapshot"`
	TotalFiles         int                `json:"total_files"`
	FailedFiles        int                `json:"failed_files"`
	VerifiedAt         string             `json:"verified_at"`
	EnvironmentContext EnvironmentContext `json:"environment_context"`
}

func VerifyFile(path string, expectedSHA256 string) error {
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != expectedSHA256 {
		return fmt.Errorf("checksum mismatch: got %s expected %s", got, expectedSHA256)
	}
	return nil
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
