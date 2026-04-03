alter table tasks
  add column if not exists asset_object_key text not null default '',
  add column if not exists media_kind text not null default 'image',
  add column if not exists frame_index integer,
  add column if not exists ontology_version text not null default 'v1';

update tasks
set status = 'closed'
where status = 'done';

update tasks
set media_kind = 'image'
where coalesce(media_kind, '') = '';

update tasks
set ontology_version = 'v1'
where coalesce(ontology_version, '') = '';

alter table tasks
  drop constraint if exists tasks_status_check;

alter table tasks
  add constraint tasks_status_check check (
    status in (
      'queued',
      'ready',
      'in_progress',
      'blocked',
      'submitted',
      'reviewing',
      'rework_required',
      'accepted',
      'published',
      'closed'
    )
  );

alter table tasks
  drop constraint if exists tasks_kind_check;

alter table tasks
  add constraint tasks_kind_check check (
    kind in ('annotation', 'review', 'qa', 'ops', 'training_candidate', 'promotion_review')
  );
