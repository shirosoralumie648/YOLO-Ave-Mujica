create table tasks (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  snapshot_id bigint references dataset_snapshots(id),
  title text not null,
  kind text not null,
  status text not null,
  priority text not null default 'normal',
  assignee text not null default '',
  due_at timestamptz,
  blocker_reason text not null default '',
  last_activity_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint tasks_kind_check check (kind in ('annotation','review','qa','ops')),
  constraint tasks_status_check check (status in ('queued','ready','in_progress','blocked','done')),
  constraint tasks_priority_check check (priority in ('low','normal','high','critical'))
);

create index idx_tasks_project_status on tasks(project_id, status);
create index idx_tasks_project_activity on tasks(project_id, last_activity_at);
