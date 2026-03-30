import { useQuery } from "@tanstack/react-query";

import { apiGet } from "../../lib/api";

export interface Task {
  id: number;
  project_id: number;
  dataset_id?: number;
  snapshot_id?: number;
  title: string;
  description?: string;
  assignee?: string;
  status: string;
  priority: string;
  due_at?: string;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
}

interface TaskListResponse {
  items: Task[];
}

export function useProjectTasks(projectId: string) {
  return useQuery({
    queryKey: ["project-tasks", projectId],
    queryFn: async () => {
      const response = await apiGet<TaskListResponse>(`/projects/${projectId}/tasks`);
      return response.items;
    },
  });
}

export function useTask(taskId: string) {
  return useQuery({
    queryKey: ["task-detail", taskId],
    queryFn: () => apiGet<Task>(`/tasks/${taskId}`),
  });
}
