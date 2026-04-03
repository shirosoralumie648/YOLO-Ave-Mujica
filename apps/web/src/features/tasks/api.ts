import { getJSON, postJSON } from "../shared/http";

export interface TaskItem {
  id: number;
  project_id: number;
  snapshot_id?: number;
  title: string;
  kind: string;
  asset_object_key?: string;
  media_kind?: string;
  frame_index?: number;
  ontology_version?: string;
  status: string;
  priority: string;
  assignee: string;
  due_at?: string;
  blocker_reason: string;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
  snapshot_version?: string;
  dataset_id?: number;
  dataset_name?: string;
}

export interface TaskListResponse {
  items: TaskItem[];
}

export interface TaskListFilter {
  status?: string;
  kind?: string;
  assignee?: string;
  priority?: string;
}

export interface CreateTaskPayload {
  title: string;
  assignee: string;
  kind: string;
  priority: string;
}

export function listTasks(projectId = 1, filter: TaskListFilter = {}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filter)) {
    if (value) {
      params.set(key, value);
    }
  }

  const query = params.toString();
  const path = query
    ? `/v1/projects/${projectId}/tasks?${query}`
    : `/v1/projects/${projectId}/tasks`;

  return getJSON<TaskListResponse>(path);
}

export function createTask(projectId = 1, payload: CreateTaskPayload) {
  return postJSON<TaskItem>(`/v1/projects/${projectId}/tasks`, payload);
}

export function getTask(taskId: number | string) {
  return getJSON<TaskItem>(`/v1/tasks/${taskId}`);
}

export function transitionTask(
  taskId: number | string,
  payload: { status: string; blocker_reason?: string },
) {
  return postJSON<TaskItem>(`/v1/tasks/${taskId}/transition`, payload);
}
