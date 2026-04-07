create table worker_instances (
  worker_id text primary key,
  resource_lane text not null,
  capabilities_json jsonb not null default '[]'::jsonb,
  job_types_json jsonb not null default '[]'::jsonb,
  registered_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  constraint worker_instances_resource_lane_check check (resource_lane in ('jobs:cpu', 'jobs:gpu', 'jobs:mixed'))
);

create index idx_worker_instances_last_seen on worker_instances(last_seen_at desc);
