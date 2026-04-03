create table publish_batches (
  id bigserial primary key,
  project_id bigint not null references projects(id) on delete cascade,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  source text not null,
  status text not null,
  rule_summary_json jsonb not null default '{}'::jsonb,
  owner_edit_version integer not null default 0,
  review_approved_at timestamptz,
  review_approved_by text not null default '',
  owner_decided_at timestamptz,
  owner_decided_by text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table publish_batch_items (
  id bigserial primary key,
  publish_batch_id bigint not null references publish_batches(id) on delete cascade,
  candidate_id bigint not null,
  task_id bigint not null,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  dataset_id bigint not null references datasets(id) on delete cascade,
  item_payload_json jsonb not null,
  position integer not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table publish_feedback (
  id bigserial primary key,
  publish_batch_id bigint not null references publish_batches(id) on delete cascade,
  publish_batch_item_id bigint references publish_batch_items(id) on delete cascade,
  scope text not null,
  stage text not null,
  action text not null,
  reason_code text not null,
  severity text not null,
  influence_weight numeric(5,2) not null default 1.00,
  comment text not null default '',
  created_by text not null,
  created_at timestamptz not null default now()
);

create table publish_records (
  id bigserial primary key,
  project_id bigint not null references projects(id) on delete cascade,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  publish_batch_id bigint not null references publish_batches(id) on delete cascade,
  status text not null,
  summary_json jsonb not null default '{}'::jsonb,
  approved_by_owner text not null,
  approved_at timestamptz not null,
  created_at timestamptz not null default now()
);

create index idx_publish_batches_snapshot_status
  on publish_batches(snapshot_id, status);

create index idx_publish_batch_items_batch_position
  on publish_batch_items(publish_batch_id, position);

create index idx_publish_feedback_batch
  on publish_feedback(publish_batch_id, created_at desc);

create index idx_publish_records_snapshot
  on publish_records(snapshot_id, approved_at desc);
