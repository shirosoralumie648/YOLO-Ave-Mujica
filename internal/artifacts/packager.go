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

type ManifestStats struct {
	TotalImages      int `json:"total_images"`
	TotalAnnotations int `json:"total_annotations"`
	TotalClasses     int `json:"total_classes"`
}

type Manifest struct {
	Version     string            `json:"version"`
	GeneratedAt time.Time         `json:"generated_at"`
	CategoryMap map[string]string `json:"category_map"`
	Stats       ManifestStats     `json:"stats"`
	Entries     []ManifestEntry   `json:"entries"`
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
	if len(names) == 0 {
		return fmt.Sprintf("train: %s\nval: %s\nnames: []\n", train, val)
	}
	return fmt.Sprintf("train: %s\nval: %s\nnames:\n  - %s\n", train, val, strings.Join(names, "\n  - "))
}

// BuildManifest creates the package integrity manifest consumed by CLI verification.
func BuildManifest(version string, entries []ManifestEntry) ([]byte, error) {
	return BuildManifestWithMetadata(version, nil, ManifestStats{}, entries)
}

func BuildManifestWithMetadata(version string, categoryMap map[string]string, stats ManifestStats, entries []ManifestEntry) ([]byte, error) {
	normalizedEntries := make([]ManifestEntry, len(entries))
	copy(normalizedEntries, entries)
	for i := range normalizedEntries {
		normalizedEntries[i].Checksum = NormalizeSHA256Checksum(normalizedEntries[i].Checksum)
	}
	payload := Manifest{
		Version:     version,
		GeneratedAt: time.Now().UTC(),
		CategoryMap: cloneCategoryMap(categoryMap),
		Stats:       stats,
		Entries:     normalizedEntries,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func NormalizeSHA256Checksum(checksum string) string {
	if checksum == "" || strings.HasPrefix(checksum, "sha256:") {
		return checksum
	}
	return "sha256:" + checksum
}

func cloneCategoryMap(categoryMap map[string]string) map[string]string {
	if len(categoryMap) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(categoryMap))
	for key, value := range categoryMap {
		cloned[key] = value
	}
	return cloned
}
