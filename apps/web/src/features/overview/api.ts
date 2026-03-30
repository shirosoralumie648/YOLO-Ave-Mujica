import { useQuery } from "@tanstack/react-query";

import { apiGet } from "../../lib/api";

export interface IdleTask {
  id: number;
  title: string;
  assignee?: string;
  status: string;
  last_activity_at: string;
}

export interface BlockerCard {
  kind: string;
  severity: string;
  title: string;
  description: string;
  href: string;
}

export interface ProjectOverview {
  project_id: number;
  open_task_count: number;
  review_backlog: number;
  failed_recent_jobs: number;
  longest_idle_task?: IdleTask;
  blockers: BlockerCard[];
}

export function useProjectOverview(projectId: string) {
  return useQuery({
    queryKey: ["project-overview", projectId],
    queryFn: () => apiGet<ProjectOverview>(`/projects/${projectId}/overview`),
  });
}
