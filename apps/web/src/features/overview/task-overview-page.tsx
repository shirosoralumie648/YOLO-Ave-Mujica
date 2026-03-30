import { Link, useParams, useSearchParams } from "react-router-dom";

import { useProjectOverview } from "./api";

export function TaskOverviewPage() {
  const { projectId = "1" } = useParams();
  const [searchParams] = useSearchParams();
  const snapshotID = searchParams.get("snapshot_id");
  const overviewQuery = useProjectOverview(projectId);
  const overview = overviewQuery.data;
  const taskQueueHref = buildProjectPath(projectId, "tasks", searchParams);

  if (overviewQuery.isLoading) {
    return <PageState title="Task Overview" description="Loading live blockers and active work." />;
  }

  if (overviewQuery.isError || !overview) {
    return (
      <PageState
        title="Task Overview"
        description="Overview data is unavailable right now. Check the API server and retry."
      />
    );
  }

  return (
    <div className="page-grid">
      <section className="panel page-intro">
        <p className="eyebrow">Task Overview</p>
        <h1>Task Overview</h1>
        <p>
          当前项目的任务入口、阻塞视图和忘记处理的工作会先集中在这里。
          {snapshotID ? ` 当前快照上下文是 ${snapshotID}。` : ""}
        </p>
        <div className="hero-actions">
          <Link className="action-link primary" to={taskQueueHref}>
            Open task queue
          </Link>
        </div>
      </section>

      <section className="summary-grid">
        <MetricCard label="Open Tasks" value={String(overview.open_task_count)} accent="amber" />
        <MetricCard label="Review Backlog" value={String(overview.review_backlog)} accent="teal" />
        <MetricCard label="Failed Jobs" value={String(overview.failed_recent_jobs)} accent="rust" />
      </section>

      <div className="two-up">
        <section className="panel content-panel">
          <div className="section-header">
            <div>
              <p className="eyebrow">Blockers View</p>
              <h2>Current bottlenecks</h2>
            </div>
            <span className="context-pill muted">{overview.blockers.length} blockers</span>
          </div>
          {overview.blockers.length > 0 ? (
            <div className="stack-list">
              {overview.blockers.map((blocker) => (
                <article key={blocker.kind} className={`blocker-card ${blocker.severity}`}>
                  <div>
                    <p className="blocker-meta">{formatSeverity(blocker.severity)}</p>
                    <h3>{blocker.title}</h3>
                    <p>{blocker.description}</p>
                  </div>
                  <a className="action-link subtle" href={blocker.href}>
                    Open handling page
                  </a>
                </article>
              ))}
            </div>
          ) : (
            <p className="empty-copy">No blockers right now. The production chain looks healthy.</p>
          )}
        </section>

        <section className="panel content-panel">
          <div className="section-header">
            <div>
              <p className="eyebrow">Longest Idle Task</p>
              <h2>Silent blocker</h2>
            </div>
          </div>
          {overview.longest_idle_task ? (
            <div className="detail-card">
              <div className="detail-card-top">
                <div>
                  <h3>{overview.longest_idle_task.title}</h3>
                  <p>{overview.longest_idle_task.assignee || "Unassigned"}</p>
                </div>
                <span className="status-pill">{overview.longest_idle_task.status}</span>
              </div>
              <p>
                Last activity at{" "}
                {formatDateTime(overview.longest_idle_task.last_activity_at)}.
              </p>
              <Link className="action-link subtle" to={taskQueueHref}>
                Open task queue
              </Link>
            </div>
          ) : (
            <p className="empty-copy">No idle task is currently tracked for this project.</p>
          )}
        </section>
      </div>
    </div>
  );
}

function MetricCard(props: { label: string; value: string; accent: "amber" | "teal" | "rust" }) {
  return (
    <article className={`panel metric-card ${props.accent}`}>
      <p className="metric-label">{props.label}</p>
      <strong>{props.value}</strong>
    </article>
  );
}

function PageState(props: { title: string; description: string }) {
  return (
    <section className="panel page-intro">
      <p className="eyebrow">Task-first Routing</p>
      <h1>{props.title}</h1>
      <p>{props.description}</p>
    </section>
  );
}

function buildProjectPath(projectId: string, section: string, searchParams: URLSearchParams): string {
  const nextSearch = searchParams.toString();
  const base = `/projects/${projectId}/${section}`;
  return nextSearch ? `${base}?${nextSearch}` : base;
}

function formatSeverity(severity: string) {
  switch (severity) {
  case "high":
    return "High Risk";
  case "medium":
    return "Needs attention";
  default:
    return severity;
  }
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
