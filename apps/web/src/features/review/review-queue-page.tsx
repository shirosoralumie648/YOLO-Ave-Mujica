import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { listTasks } from "../tasks/api";

export function ReviewQueuePage() {
  const queueQuery = useQuery({
    queryKey: ["review-queue", 1],
    queryFn: () => listTasks(1, { kind: "review" }),
  });

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Review Queue</h1>
          <p className="page-summary">Process review and publish work without leaving the operations shell.</p>
        </div>
      </header>

      {queueQuery.isLoading ? <p>Loading review queue.</p> : null}
      {queueQuery.isError ? <p role="alert">Failed to load review queue: {queueQuery.error.message}</p> : null}

      {queueQuery.data ? (
        queueQuery.data.items.length === 0 ? (
          <section className="panel panel-accent">
            <p>No review tasks are active right now.</p>
          </section>
        ) : (
          <section className="panel">
            <div className="panel-header">
              <h2>Active review work</h2>
              <span>{queueQuery.data.items.length}</span>
            </div>
            <div className="stack-list">
              {queueQuery.data.items.map((task) => (
                <Link className="stack-item" key={task.id} to={`/publish/batches/${task.id}`}>
                  <strong>{task.title}</strong>
                  <div className="stack-item__meta">
                    <span>{task.status}</span>
                    <span>{task.snapshot_id ? `Snapshot ${task.snapshot_id}` : "Project-scoped"}</span>
                  </div>
                </Link>
              ))}
            </div>
          </section>
        )
      ) : null}
    </section>
  );
}
