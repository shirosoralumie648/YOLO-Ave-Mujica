drop index if exists idx_artifacts_format_version_ready;

alter table artifacts
  drop column if exists error_msg;
