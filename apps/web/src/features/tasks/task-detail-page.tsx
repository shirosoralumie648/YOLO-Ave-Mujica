import { Link, useParams, useSearchParams } from "react-router-dom";

import { useTask } from "./api";

export function TaskDetailPage() {
  const { projectId = "1", taskId = "" } = useParams();
  const [searchParams] = useSearchParams();
  const taskQuery = useTask(taskId);
  const task = taskQuery.data;

  if (taskQuery.isLoading) {
    return <TaskDetailState title="Task Detail" description="Loading task lineage and assignment context." />;
  }

  if (taskQuery.isError || !task) {
    return <TaskDetailState title="Task Detail" description="Task detail is unavailable right now." />;
  }

  return (
    <div className="page-grid">
      <section className="panel page-intro compact">
        <p className="eyebrow">Task Detail</p>
        <h1>{task.title}</h1>
        <p>{task.description || "No task description provided."}</p>
        <div className="chip-row">
          <span className="status-pill">{task.status}</span>
          <span className="context-pill muted">{task.priority}</span>
          <span className="context-pill muted">{task.assignee || "Unassigned"}</span>
        </div>
      </section>

      <div className="two-up">
        <section className="panel content-panel">
          <div className="section-header">
            <div>
              <p className="eyebrow">Lineage Context</p>
              <h2>Version anchors</h2>
            </div>
          </div>
          <div className="chip-row">
            {task.dataset_id ? <span className="context-pill">Dataset {task.dataset_id}</span> : null}
            {task.snapshot_id ? <span className="context-pill">Snapshot {task.snapshot_id}</span> : null}
            <span className="context-pill muted">Project {task.project_id}</span>
          </div>
          <p className="detail-copy">
            Created {formatDateTime(task.created_at)} and last updated {formatDateTime(task.updated_at)}.
          </p>
        </section>

        <section className="panel content-panel">
          <div className="section-header">
            <div>
              <p className="eyebrow">Next Action</p>
              <h2>Return path</h2>
            </div>
          </div>
          <p className="detail-copy">
            Return to the queue with the same snapshot context so follow-up coordination stays shareable.
          </p>
          <Link className="action-link primary" to={buildTaskListPath(projectId, searchParams)}>
            Back to task queue
          </Link>
        </section>
      </div>
    </div>
  );
}

function TaskDetailState(props: { title: string; description: string }) {
  return (
    <section className="panel page-intro compact">
      <p className="eyebrow">Task Detail</p>
      <h1>{props.title}</h1>
      <p>{props.description}</p>
    </section>
  );
}

function buildTaskListPath(projectId: string, searchParams: URLSearchParams): string {
  const nextSearch = searchParams.toString();
  const base = `/projects/${projectId}/tasks`;
  return nextSearch ? `${base}?${nextSearch}` : base;
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
