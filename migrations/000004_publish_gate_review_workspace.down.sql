drop index if exists idx_publish_records_snapshot;
drop index if exists idx_publish_feedback_batch;
drop index if exists idx_publish_batch_items_batch_position;
drop index if exists idx_publish_batches_snapshot_status;

drop table if exists publish_records;
drop table if exists publish_feedback;
drop table if exists publish_batch_items;
drop table if exists publish_batches;
