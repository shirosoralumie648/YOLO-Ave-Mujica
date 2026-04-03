import { getJSON, postJSON, putJSON } from "../shared/http";
import type { TaskItem } from "../tasks/api";

export interface AnnotationWorkspaceAsset {
  dataset_id?: number;
  dataset_name?: string;
  snapshot_id?: number;
  snapshot_version?: string;
  object_key: string;
  frame_index?: number;
}

export interface AnnotationDraft {
  id?: number;
  task_id?: number;
  state: string;
  revision: number;
  body: Record<string, unknown>;
  updated_at?: string;
}

export interface AnnotationWorkspace {
  task: TaskItem;
  asset: AnnotationWorkspaceAsset;
  draft: AnnotationDraft;
}

export interface SaveAnnotationWorkspaceDraftPayload {
  actor: string;
  base_revision?: number;
  body: Record<string, unknown>;
}

export function getAnnotationWorkspace(taskId: number | string) {
  return getJSON<AnnotationWorkspace>(`/v1/tasks/${taskId}/workspace`);
}

export function saveAnnotationWorkspaceDraft(
  taskId: number | string,
  payload: SaveAnnotationWorkspaceDraftPayload,
) {
  return putJSON<AnnotationWorkspace>(`/v1/tasks/${taskId}/workspace/draft`, payload);
}

export function submitAnnotationWorkspace(taskId: number | string, actor: string) {
  return postJSON<AnnotationWorkspace>(`/v1/tasks/${taskId}/workspace/submit`, { actor });
}
