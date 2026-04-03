import { getJSON, postJSON } from "../shared/http";

export interface SuggestedPublishGroup {
  snapshot_id: number;
  suggestion_key: string;
  summary: Record<string, unknown>;
  items: Array<{
    candidate_id: number;
    task_id: number;
    dataset_id: number;
    item_payload: Record<string, unknown>;
  }>;
}

export interface PublishBatchItem {
  id: number;
  candidate_id: number;
  task_id: number;
  dataset_id: number;
  snapshot_id: number;
  item_payload: Record<string, unknown>;
}

export interface PublishFeedback {
  id: number;
  scope: string;
  stage: string;
  action: string;
  reason_code: string;
  severity: string;
  influence_weight: number;
  comment: string;
  publish_batch_item_id?: number;
}

export interface PublishBatch {
  id: number;
  snapshot_id: number;
  project_id: number;
  status: string;
  source: string;
  rule_summary: Record<string, unknown>;
  items: PublishBatchItem[];
  feedback: PublishFeedback[];
}

export interface PublishWorkspace {
  batch: PublishBatch;
  items: Array<{
    item_id: number;
    candidate_id: number;
    task_id: number;
    overlay: Record<string, unknown>;
    diff: Record<string, unknown>;
    context?: Record<string, unknown>;
    feedback: PublishFeedback[];
  }>;
  history: Array<{ stage: string; actor: string; action: string; at?: string }>;
}

export interface CreateFeedbackPayload {
  stage: string;
  action: string;
  reason_code: string;
  severity: string;
  influence_weight: number;
  comment?: string;
  actor: string;
}

export function listPublishCandidates(projectId = 1) {
  return getJSON<{ items: SuggestedPublishGroup[] }>(`/v1/publish/candidates?project_id=${projectId}`);
}

export function createPublishBatch(payload: unknown) {
  return postJSON<PublishBatch>("/v1/publish/batches", payload);
}

export function getPublishBatch(batchId: number | string) {
  return getJSON<PublishBatch>(`/v1/publish/batches/${batchId}`);
}

export function getPublishWorkspace(batchId: number | string) {
  return getJSON<PublishWorkspace>(`/v1/publish/batches/${batchId}/workspace`);
}

export function reviewApprove(batchId: number | string, actor: string) {
  return postJSON<{ ok: true }>(`/v1/publish/batches/${batchId}/review-approve`, { actor });
}

export function ownerApprove(batchId: number | string, actor: string) {
  return postJSON<{ publish_record_id: number }>(`/v1/publish/batches/${batchId}/owner-approve`, { actor });
}

export function addBatchFeedback(batchId: number | string, payload: CreateFeedbackPayload) {
  return postJSON<PublishFeedback>(`/v1/publish/batches/${batchId}/feedback`, payload);
}

export function addItemFeedback(batchId: number | string, itemId: number | string, payload: CreateFeedbackPayload) {
  return postJSON<PublishFeedback>(`/v1/publish/batches/${batchId}/items/${itemId}/feedback`, payload);
}
