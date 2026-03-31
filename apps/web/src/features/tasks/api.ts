import { getJSON } from "../shared/http";

export type Task = {
  id: number;
  title: string;
  status: string;
  priority: string;
  assignee: string;
  blocker_reason?: string;
};

export function fetchTasks(projectId: string) {
  return getJSON<{ items: Task[] }>(`/v1/projects/${projectId}/tasks`);
}

export function fetchTask(taskId: string) {
  return getJSON<Task>(`/v1/tasks/${taskId}`);
}
