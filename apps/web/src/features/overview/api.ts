import { getJSON } from "../shared/http";
import type { TaskItem } from "../tasks/api";

export interface SummaryCard {
  id: string;
  title: string;
  count: number;
  href: string;
}

export interface BlockerCard {
  id: string;
  title: string;
  reason: string;
  href: string;
}

export interface FailedJobItem {
  id: number;
  job_type: string;
  status: string;
  error_msg: string;
}

export interface ProjectOverview {
  summary_cards: SummaryCard[];
  blockers: BlockerCard[];
  longest_idle_task?: TaskItem;
  recent_failed_jobs: FailedJobItem[];
}

export function getProjectOverview(projectId = 1) {
  return getJSON<ProjectOverview>(`/v1/projects/${projectId}/overview`);
}
