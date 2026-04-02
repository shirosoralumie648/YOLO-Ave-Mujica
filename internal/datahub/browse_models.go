package datahub

type DatasetSummary struct {
	ID                    int64  `json:"id"`
	ProjectID             int64  `json:"project_id"`
	Name                  string `json:"name"`
	Bucket                string `json:"bucket"`
	Prefix                string `json:"prefix"`
	ItemCount             int    `json:"item_count"`
	SnapshotCount         int    `json:"snapshot_count"`
	LatestSnapshotID      int64  `json:"latest_snapshot_id"`
	LatestSnapshotVersion string `json:"latest_snapshot_version"`
}

type DatasetDetail struct {
	ID                    int64  `json:"id"`
	ProjectID             int64  `json:"project_id"`
	Name                  string `json:"name"`
	Bucket                string `json:"bucket"`
	Prefix                string `json:"prefix"`
	ItemCount             int    `json:"item_count"`
	SnapshotCount         int    `json:"snapshot_count"`
	LatestSnapshotID      int64  `json:"latest_snapshot_id"`
	LatestSnapshotVersion string `json:"latest_snapshot_version"`
}

type SnapshotDetail struct {
	ID                int64  `json:"id"`
	DatasetID         int64  `json:"dataset_id"`
	Version           string `json:"version"`
	ProjectID         int64  `json:"project_id"`
	DatasetName       string `json:"dataset_name"`
	BasedOnSnapshotID *int64 `json:"based_on_snapshot_id,omitempty"`
	Note              string `json:"note,omitempty"`
	AnnotationCount   int    `json:"annotation_count"`
}
