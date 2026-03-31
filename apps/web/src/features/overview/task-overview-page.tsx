import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { fetchOverview } from "./api";

export function TaskOverviewPage() {
  const { projectId = "1" } = useParams();
  const overview = useQuery({
    queryKey: ["overview", projectId],
    queryFn: () => fetchOverview(projectId),
  });

  if (overview.isLoading) {
    return <section><h1>Task Overview</h1><p>Loading overview...</p></section>;
  }

  if (overview.isError || !overview.data) {
    return <section><h1>Task Overview</h1><p>Failed to load overview.</p></section>;
  }

  const data = overview.data;
  return (
    <section className="overview-page">
      <header className="page-header">
        <h1>Task Overview</h1>
        <p>Project {projectId} task-first entry.</p>
      </header>

      <div className="summary-grid">
        <article><h2>Open Tasks</h2><p>{data.open_task_count}</p></article>
        <article><h2>Blocked Tasks</h2><p>{data.blocked_task_count}</p></article>
        <article><h2>Review Backlog</h2><p>{data.review_backlog_count}</p></article>
        <article><h2>Failed Jobs</h2><p>{data.failed_recent_jobs}</p></article>
      </div>

      <section>
        <h2>Blockers View</h2>
        <ul>
          {data.blockers.map((blocker) => (
            <li key={blocker.task_id}>
              <Link to={`/projects/${projectId}/tasks/${blocker.task_id}`}>{blocker.title}</Link>
              <span> {blocker.reason}</span>
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h2>Longest Idle Task</h2>
        {data.longest_idle_task ? (
          <Link to={`/projects/${projectId}/tasks/${data.longest_idle_task.id}`}>
            {data.longest_idle_task.title} ({data.longest_idle_task.status})
          </Link>
        ) : (
          <p>No active tasks.</p>
        )}
      </section>
    </section>
  );
}
