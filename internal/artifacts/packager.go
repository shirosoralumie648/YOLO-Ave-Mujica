package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ManifestEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}

type BundleEntry struct {
	Path     string `json:"path"`
	Body     []byte `json:"body"`
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

func BuildBundleEntries(a Artifact) []BundleEntry {
	entries := []BundleEntry{
		newBundleEntry("data.yaml", []byte(BuildDataYAML("images/train", "images/val", bundleNames(a.LabelMapJSON)))),
		newBundleEntry("labels/0001.txt", []byte("0 0.5 0.5 0.2 0.2\n")),
	}

	if a.Format != "yolo" {
		entries = append(entries, newBundleEntry("README.txt", []byte(fmt.Sprintf("artifact format=%s version=%s\n", a.Format, a.Version))))
		return entries
	}

	entries = append(entries, newBundleEntry("images/README.txt", []byte(fmt.Sprintf("dataset=%d snapshot=%d artifact=%d\n", a.DatasetID, a.SnapshotID, a.ID))))
	return entries
}

func bundleNames(labelMap map[string]string) []string {
	if len(labelMap) == 0 {
		return []string{"person"}
	}

	seen := make(map[string]struct{}, len(labelMap))
	names := make([]string, 0, len(labelMap))
	for _, label := range labelMap {
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		names = append(names, label)
	}
	if len(names) == 0 {
		return []string{"person"}
	}
	sort.Strings(names)
	return names
}

func newBundleEntry(path string, body []byte) BundleEntry {
	sum := sha256.Sum256(body)
	return BundleEntry{
		Path:     path,
		Body:     body,
		Checksum: hex.EncodeToString(sum[:]),
	}
}
