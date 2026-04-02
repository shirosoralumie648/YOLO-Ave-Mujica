import { getJSON, postJSON } from "../shared/http";

export interface DatasetSummary {
  id: number;
  project_id: number;
  name: string;
  bucket: string;
  prefix: string;
  item_count: number;
  snapshot_count: number;
  latest_snapshot_id?: number;
  latest_snapshot_version?: string;
}

export interface DatasetDetail {
  id: number;
  project_id: number;
  name: string;
  bucket: string;
  prefix: string;
  item_count: number;
  snapshot_count: number;
  latest_snapshot_id?: number;
  latest_snapshot_version?: string;
}

export interface DatasetItem {
  id: number;
  dataset_id: number;
  object_key: string;
  etag: string;
}

export interface SnapshotItem {
  id: number;
  dataset_id: number;
  version: string;
  based_on_snapshot_id?: number;
  note?: string;
}

export interface SnapshotDetail {
  id: number;
  dataset_id: number;
  dataset_name: string;
  project_id: number;
  version: string;
  based_on_snapshot_id?: number;
  note?: string;
  annotation_count: number;
}

export interface SnapshotDiffResponse {
  adds: Array<{ item_id?: number; annotation_id?: number; category_id?: number; iou?: number }>;
  removes: Array<{ item_id?: number; annotation_id?: number; category_id?: number; iou?: number }>;
  updates: Array<{ item_id?: number; annotation_id?: number; category_id?: number; iou?: number }>;
  stats: {
    added?: number;
    removed?: number;
    updated?: number;
    added_count?: number;
    removed_count?: number;
    updated_count?: number;
  };
  compatibility_score: number;
}

interface DatasetListResponse {
  items: DatasetSummary[];
}

interface DatasetItemListResponse {
  items: DatasetItem[];
}

interface SnapshotItemListResponse {
  items: SnapshotItem[];
}

export function listDatasets() {
  return getJSON<DatasetListResponse>("/v1/datasets");
}

export function getDatasetDetail(datasetId: number | string) {
  return getJSON<DatasetDetail>(`/v1/datasets/${datasetId}`);
}

export function listDatasetItems(datasetId: number | string) {
  return getJSON<DatasetItemListResponse>(`/v1/datasets/${datasetId}/items`);
}

export function listDatasetSnapshots(datasetId: number | string) {
  return getJSON<SnapshotItemListResponse>(`/v1/datasets/${datasetId}/snapshots`);
}

export function getSnapshotDetail(snapshotId: number | string) {
  return getJSON<SnapshotDetail>(`/v1/snapshots/${snapshotId}`);
}

export function diffSnapshots(beforeSnapshotId: number, afterSnapshotId: number) {
  return postJSON<SnapshotDiffResponse>("/v1/snapshots/diff", {
    before_snapshot_id: beforeSnapshotId,
    after_snapshot_id: afterSnapshotId,
  });
}
