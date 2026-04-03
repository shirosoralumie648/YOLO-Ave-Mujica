update tasks
set status = 'done'
where status in ('submitted', 'reviewing', 'rework_required', 'accepted', 'published', 'closed');

update tasks
set kind = 'review'
where kind in ('training_candidate', 'promotion_review');

alter table tasks
  drop constraint if exists tasks_status_check;

alter table tasks
  add constraint tasks_status_check check (
    status in ('queued', 'ready', 'in_progress', 'blocked', 'done')
  );

alter table tasks
  drop constraint if exists tasks_kind_check;

alter table tasks
  add constraint tasks_kind_check check (
    kind in ('annotation', 'review', 'qa', 'ops')
  );

alter table tasks
  drop column if exists ontology_version,
  drop column if exists frame_index,
  drop column if exists media_kind,
  drop column if exists asset_object_key;
