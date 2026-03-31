import { getJSON } from "../shared/http";

export type BlockerCard = {
  task_id: number;
  title: string;
  reason: string;
  status: string;
  minutes_idle: number;
};

export type OverviewResponse = {
  open_task_count: number;
  blocked_task_count: number;
  review_backlog_count: number;
  failed_recent_jobs: number;
  blockers: BlockerCard[];
  longest_idle_task?: {
    id: number;
    title: string;
    status: string;
  };
};

export function fetchOverview(projectId: string) {
  return getJSON<OverviewResponse>(`/v1/projects/${projectId}/overview`);
}
