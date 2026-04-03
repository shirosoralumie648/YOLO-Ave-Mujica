create table task_annotations (
  id bigserial primary key,
  task_id bigint not null references tasks(id) on delete cascade,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  asset_object_key text not null,
  frame_index integer,
  ontology_version text not null default 'v1',
  state text not null default 'draft',
  revision bigint not null default 1,
  body_json jsonb not null default '{}'::jsonb,
  submitted_by text not null default '',
  submitted_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (task_id)
);

create or replace function ensure_task_annotation_context_match()
returns trigger as $$
declare
  task_snapshot_id bigint;
  task_asset_object_key text;
  task_frame_index integer;
  task_ontology_version text;
begin
  select snapshot_id, asset_object_key, frame_index, ontology_version
    into task_snapshot_id, task_asset_object_key, task_frame_index, task_ontology_version
  from tasks
  where id = new.task_id;

  if task_snapshot_id is distinct from new.snapshot_id
     or task_asset_object_key is distinct from new.asset_object_key
     or task_frame_index is distinct from new.frame_index
     or task_ontology_version is distinct from new.ontology_version then
    raise exception 'task_annotations context mismatch for task_id=%', new.task_id;
  end if;

  return new;
end;
$$ language plpgsql;

create trigger task_annotations_context_check
before insert or update on task_annotations
for each row
execute function ensure_task_annotation_context_match();
