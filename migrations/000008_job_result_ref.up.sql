alter table jobs
add column result_ref_json jsonb not null default '{}'::jsonb;
