import { Link, useParams, useSearchParams } from "react-router-dom";

import { useProjectTasks } from "./api";

export function TaskListPage() {
  const { projectId = "1" } = useParams();
  const [searchParams] = useSearchParams();
  const tasksQuery = useProjectTasks(projectId);
  const items = tasksQuery.data ?? [];

  if (tasksQuery.isLoading) {
    return <TaskListState title="Task Queue" description="Loading task assignments." />;
  }

  if (tasksQuery.isError) {
    return <TaskListState title="Task Queue" description="Task list is unavailable right now." />;
  }

  return (
    <div className="page-grid">
      <section className="panel page-intro compact">
        <p className="eyebrow">Project Tasks</p>
        <h1>Task Queue</h1>
        <p>任务列表以责任、优先级和版本上下文来组织，优先暴露需要立即处理的工作。</p>
      </section>

      <section className="panel content-panel">
        <div className="section-header">
          <div>
            <p className="eyebrow">Live Queue</p>
            <h2>{items.length} active items</h2>
          </div>
          <span className="context-pill muted">Project {projectId}</span>
        </div>

        <div className="stack-list">
          {items.map((item) => (
            <article key={item.id} className="task-row">
              <div className="task-row-main">
                <div className="task-row-top">
                  <h3>{item.title}</h3>
                  <span className="status-pill">{item.status}</span>
                </div>
                <p>{item.description || "No task description provided."}</p>
                <div className="chip-row">
                  <span className="context-pill muted">{item.assignee || "Unassigned"}</span>
                  <span className="context-pill muted">{item.priority}</span>
                  {item.snapshot_id ? <span className="context-pill muted">Snapshot {item.snapshot_id}</span> : null}
                </div>
              </div>
              <Link
                className="action-link subtle"
                to={buildTaskDetailPath(projectId, item.id, searchParams)}
              >
                Open task detail
              </Link>
            </article>
          ))}
          {items.length === 0 ? <p className="empty-copy">No tasks created for this project yet.</p> : null}
        </div>
      </section>
    </div>
  );
}

function TaskListState(props: { title: string; description: string }) {
  return (
    <section className="panel page-intro compact">
      <p className="eyebrow">Project Tasks</p>
      <h1>{props.title}</h1>
      <p>{props.description}</p>
    </section>
  );
}

function buildTaskDetailPath(projectId: string, taskId: number, searchParams: URLSearchParams): string {
  const nextSearch = searchParams.toString();
  const base = `/projects/${projectId}/tasks/${taskId}`;
  return nextSearch ? `${base}?${nextSearch}` : base;
}
