create table tasks (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  snapshot_id bigint references dataset_snapshots(id),
  title text not null,
  kind text not null default 'annotation',
  status text not null default 'queued',
  priority text not null default 'normal',
  assignee text not null default '',
  due_at timestamptz,
  blocker_reason text not null default '',
  last_activity_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint tasks_kind_check check (kind in ('annotation', 'review', 'qa', 'ops')),
  constraint tasks_status_check check (status in ('queued', 'ready', 'in_progress', 'blocked', 'done')),
  constraint tasks_priority_check check (priority in ('low', 'normal', 'high', 'critical'))
);

create index idx_tasks_project_status_kind_priority
  on tasks(project_id, status, kind, priority);

create index idx_tasks_project_snapshot
  on tasks(project_id, snapshot_id);

create index idx_tasks_project_assignee
  on tasks(project_id, assignee);

create or replace function ensure_tasks_snapshot_project_match()
returns trigger
language plpgsql
as $$
declare
  snapshot_project_id bigint;
begin
  if new.snapshot_id is null then
    return new;
  end if;

  select datasets.project_id
    into snapshot_project_id
  from dataset_snapshots
  join datasets on datasets.id = dataset_snapshots.dataset_id
  where dataset_snapshots.id = new.snapshot_id;

  if snapshot_project_id is null then
    raise exception 'tasks.snapshot_id % does not exist', new.snapshot_id;
  end if;

  if snapshot_project_id <> new.project_id then
    raise exception 'tasks.snapshot_id % belongs to project %, not %', new.snapshot_id, snapshot_project_id, new.project_id;
  end if;

  return new;
end;
$$;

create trigger trg_tasks_snapshot_project_match
before insert or update of project_id, snapshot_id on tasks
for each row
execute function ensure_tasks_snapshot_project_match();
