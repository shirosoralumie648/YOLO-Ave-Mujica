create table projects (
  id bigserial primary key,
  name text not null,
  owner text not null,
  created_at timestamptz not null default now()
);

create table datasets (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  name text not null,
  storage_type text not null default 's3',
  bucket text not null,
  prefix text not null,
  created_at timestamptz not null default now()
);

create table dataset_items (
  id bigserial primary key,
  dataset_id bigint not null references datasets(id),
  object_key text not null,
  etag text,
  size bigint,
  width int,
  height int,
  mime text,
  discovered_at timestamptz not null default now(),
  unique(dataset_id, object_key)
);

create table dataset_snapshots (
  id bigserial primary key,
  dataset_id bigint not null references datasets(id),
  version text not null,
  based_on_snapshot_id bigint references dataset_snapshots(id),
  created_by text not null,
  created_at timestamptz not null default now(),
  note text,
  unique(dataset_id, version)
);

create table categories (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  name text not null,
  alias_group text,
  color text,
  unique(project_id, name)
);

create table annotations (
  id bigserial primary key,
  dataset_id bigint not null references datasets(id),
  item_id bigint not null references dataset_items(id),
  category_id bigint not null references categories(id),
  bbox_x double precision not null,
  bbox_y double precision not null,
  bbox_w double precision not null,
  bbox_h double precision not null,
  polygon_json jsonb,
  source text not null default 'manual',
  model_name text,
  created_at_snapshot_id bigint not null references dataset_snapshots(id),
  deleted_at_snapshot_id bigint references dataset_snapshots(id),
  review_status text not null default 'verified',
  is_pseudo boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table annotation_candidates (
  id bigserial primary key,
  job_id bigint,
  dataset_id bigint not null references datasets(id),
  snapshot_id bigint not null references dataset_snapshots(id),
  item_id bigint not null references dataset_items(id),
  category_id bigint not null references categories(id),
  bbox_x double precision not null,
  bbox_y double precision not null,
  bbox_w double precision not null,
  bbox_h double precision not null,
  polygon_json jsonb,
  confidence double precision,
  model_name text,
  is_pseudo boolean not null default true,
  review_status text not null default 'pending',
  reviewer_id text,
  reviewed_at timestamptz,
  created_at timestamptz not null default now()
);

create table annotation_changes (
  id bigserial primary key,
  from_snapshot_id bigint not null references dataset_snapshots(id),
  to_snapshot_id bigint not null references dataset_snapshots(id),
  item_id bigint not null references dataset_items(id),
  change_type text not null,
  before_json jsonb,
  after_json jsonb,
  created_at timestamptz not null default now()
);

create table jobs (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  dataset_id bigint references datasets(id),
  snapshot_id bigint references dataset_snapshots(id),
  job_type text not null,
  status text not null,
  priority text not null default 'normal',
  required_resource_type text not null,
  required_capabilities_json jsonb not null default '[]'::jsonb,
  idempotency_key text not null,
  worker_id text,
  payload_json jsonb not null,
  result_artifact_ids_json jsonb not null default '[]'::jsonb,
  total_items int not null default 0,
  succeeded_items int not null default 0,
  failed_items int not null default 0,
  error_code text,
  error_msg text,
  retry_count int not null default 0,
  lease_until timestamptz,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  constraint jobs_status_check check (status in (
    'queued','running','succeeded','succeeded_with_errors','failed','canceled','retry_waiting'
  )),
  constraint jobs_resource_check check (required_resource_type in ('cpu','gpu','mixed')),
  unique(project_id, job_type, idempotency_key)
);

create table job_events (
  id bigserial primary key,
  job_id bigint not null references jobs(id),
  item_id bigint references dataset_items(id),
  event_level text not null,
  event_type text not null,
  message text not null,
  detail_json jsonb not null default '{}'::jsonb,
  ts timestamptz not null default now()
);

create table artifacts (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  dataset_id bigint not null references datasets(id),
  snapshot_id bigint not null references dataset_snapshots(id),
  artifact_type text not null,
  format text not null,
  version text not null,
  uri text not null,
  checksum text not null,
  size bigint not null,
  manifest_uri text not null,
  label_map_json jsonb not null default '{}'::jsonb,
  status text not null,
  ttl_expire_at timestamptz,
  created_by_job_id bigint references jobs(id),
  created_at timestamptz not null default now()
);

create table audit_logs (
  id bigserial primary key,
  actor text not null,
  action text not null,
  resource_type text not null,
  resource_id text not null,
  detail_json jsonb not null default '{}'::jsonb,
  ts timestamptz not null default now()
);

insert into projects (id, name, owner)
values (1, 'default', 'system')
on conflict (id) do nothing;

select setval('projects_id_seq', coalesce((select max(id) from projects), 1), true);

create index idx_dataset_items_dataset on dataset_items(dataset_id);
create index idx_annotations_interval on annotations(dataset_id, created_at_snapshot_id, deleted_at_snapshot_id);
create index idx_jobs_status_resource on jobs(status, required_resource_type);
create index idx_job_events_job on job_events(job_id);
