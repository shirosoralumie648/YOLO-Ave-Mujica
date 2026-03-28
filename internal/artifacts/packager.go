package artifacts

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ManifestEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}

type Manifest struct {
	Version     string          `json:"version"`
	GeneratedAt time.Time       `json:"generated_at"`
	Entries     []ManifestEntry `json:"entries"`
}

// ApplyLabelMap remaps export labels without mutating canonical stored annotations.
func ApplyLabelMap(input []string, m map[string]string) []string {
	out := make([]string, len(input))
	for i, v := range input {
		if mv, ok := m[v]; ok {
			out[i] = mv
			continue
		}
		out[i] = v
	}
	return out
}

// BuildDataYAML emits a minimal YOLO-compatible config for pulled artifacts.
func BuildDataYAML(train, val string, names []string) string {
	return fmt.Sprintf("train: %s\nval: %s\nnames:\n  - %s\n", train, val, strings.Join(names, "\n  - "))
}

// BuildManifest creates the package integrity manifest consumed by CLI verification.
func BuildManifest(version string, entries []ManifestEntry) ([]byte, error) {
	payload := Manifest{Version: version, Entries: entries, GeneratedAt: time.Now().UTC()}
	return json.MarshalIndent(payload, "", "  ")
}
