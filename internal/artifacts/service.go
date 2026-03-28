package artifacts

import "time"

type PackageRequest struct {
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
}

type Artifact struct {
	ID           int64             `json:"id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	URI          string            `json:"uri"`
	ManifestURI  string            `json:"manifest_uri"`
	Checksum     string            `json:"checksum"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

type Service struct{}

func NewService() *Service {
	return &Service{}
}
