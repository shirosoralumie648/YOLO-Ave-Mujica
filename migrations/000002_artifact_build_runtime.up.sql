alter table artifacts
  add column if not exists error_msg text not null default '';

create index if not exists idx_artifacts_format_version_ready
  on artifacts(format, version)
  where status = 'ready';
