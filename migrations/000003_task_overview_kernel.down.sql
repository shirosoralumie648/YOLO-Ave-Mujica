drop trigger if exists trg_tasks_snapshot_project_match on tasks;
drop function if exists ensure_tasks_snapshot_project_match();

drop index if exists idx_tasks_project_assignee;
drop index if exists idx_tasks_project_snapshot;
drop index if exists idx_tasks_project_status_kind_priority;

drop table if exists tasks;
