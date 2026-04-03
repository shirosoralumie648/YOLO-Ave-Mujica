import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { getProjectOverview } from "./api";

function formatRelativeTime(iso: string | undefined) {
  if (!iso) {
    return "No recent activity";
  }

  const diffMs = Date.now() - new Date(iso).getTime();
  const diffHours = Math.max(1, Math.round(diffMs / (1000 * 60 * 60)));
  if (diffHours < 24) {
    return `${diffHours}h idle`;
  }

  const diffDays = Math.round(diffHours / 24);
  return `${diffDays}d idle`;
}

function toTitleCase(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

export function TaskOverviewPage() {
  const overviewQuery = useQuery({
    queryKey: ["overview", 1],
    queryFn: () => getProjectOverview(1),
  });

  return (
    <section className="page-stack">
      <header className="page-hero">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Task Overview</h1>
          <p className="page-summary">
            Review the current queue, unblock urgent work, and keep the annotation flow moving.
          </p>
        </div>
        <div className="hero-meter">
          <span>Control Plane</span>
          <strong>{overviewQuery.data?.summary_cards[0]?.count ?? 0}</strong>
          <small>open tasks in the active project</small>
        </div>
      </header>

      {overviewQuery.isLoading ? <p>Loading overview.</p> : null}
      {overviewQuery.isError ? (
        <p role="alert">Failed to load overview: {overviewQuery.error.message}</p>
      ) : null}

      {overviewQuery.data ? (
        <>
          <div className="summary-grid">
            {overviewQuery.data.summary_cards.map((card) => (
              <Link className="panel summary-card" key={card.id} to={card.href}>
                <span className="summary-card__title">{card.title}</span>
                <strong className="summary-card__count">{card.count}</strong>
                <small>Open filtered queue</small>
              </Link>
            ))}
          </div>

          <div className="overview-layout">
            <section className="panel">
              <div className="panel-header">
                <h2>Active blockers</h2>
                <span>{overviewQuery.data.blockers.length}</span>
              </div>
              {overviewQuery.data.blockers.length === 0 ? (
                <p>No blockers are currently recorded.</p>
              ) : (
                <div className="stack-list">
                  {overviewQuery.data.blockers.map((blocker) => (
                    <Link className="stack-item" key={blocker.id} to={blocker.href}>
                      <strong>{blocker.title}</strong>
                      <span>{blocker.reason}</span>
                    </Link>
                  ))}
                </div>
              )}
            </section>

            <section className="panel panel-accent">
              <div className="panel-header">
                <h2>Longest idle task</h2>
                <span>Needs a push</span>
              </div>
              {overviewQuery.data.longest_idle_task ? (
                <div className="idle-card">
                  <strong>
                    <Link to={`/tasks/${overviewQuery.data.longest_idle_task.id}`}>
                      {overviewQuery.data.longest_idle_task.title}
                    </Link>
                  </strong>
                  <p>
                    {toTitleCase(overviewQuery.data.longest_idle_task.status)} ·{" "}
                    {toTitleCase(overviewQuery.data.longest_idle_task.priority)}
                  </p>
                  <p>
                    {overviewQuery.data.longest_idle_task.assignee || "Unassigned"} ·{" "}
                    {formatRelativeTime(overviewQuery.data.longest_idle_task.last_activity_at)}
                  </p>
                </div>
              ) : (
                <p>No open task is currently idle.</p>
              )}
            </section>

            <section className="panel">
              <div className="panel-header">
                <h2>Recent failed jobs</h2>
                <span>{overviewQuery.data.recent_failed_jobs.length}</span>
              </div>
              {overviewQuery.data.recent_failed_jobs.length === 0 ? (
                <p>No failed jobs reported recently.</p>
              ) : (
                <div className="stack-list">
                  {overviewQuery.data.recent_failed_jobs.map((job) => (
                    <article className="stack-item" key={job.id}>
                      <strong>{job.job_type}</strong>
                      <span>{toTitleCase(job.status)}</span>
                      <span>{job.error_msg}</span>
                    </article>
                  ))}
                </div>
              )}
            </section>
          </div>
        </>
      ) : null}
    </section>
  );
}
