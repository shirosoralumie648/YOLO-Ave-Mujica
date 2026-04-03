import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { getPublishWorkspace } from "../publish/api";

export function ReviewWorkspacePage() {
  const { batchId = "" } = useParams();
  const workspaceQuery = useQuery({
    queryKey: ["publish-workspace", batchId],
    queryFn: () => getPublishWorkspace(batchId),
  });

  if (workspaceQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Review Workspace</h1>
        <p>Loading review workspace.</p>
      </section>
    );
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <section className="page-stack">
        <h1>Review Workspace</h1>
        <p role="alert">Failed to load review workspace.</p>
      </section>
    );
  }

  const workspace = workspaceQuery.data;
  const items = workspace.items ?? [];
  const history = workspace.history ?? [];

  return (
    <section className="page-stack">
      <header className="page-hero">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Review Workspace</h1>
          <p className="page-summary">
            Inspect overlay, diff context, and structured feedback before final publish decisions.
          </p>
        </div>
      </header>

      <div className="detail-grid">
        <section className="panel">
          <div className="panel-header">
            <h2>Preview And Overlay</h2>
            <span>{items.length} items</span>
          </div>
          <div className="stack-list">
            {items.map((item) => (
              <article className="stack-item" key={item.item_id}>
                <strong>Candidate #{item.candidate_id}</strong>
                <div className="stack-item__meta">
                  <span>Task #{item.task_id}</span>
                  <span>{item.feedback.length} feedback entries</span>
                </div>
                <pre className="workspace-preview">{JSON.stringify(item.overlay, null, 2)}</pre>
              </article>
            ))}
          </div>
        </section>

        <section className="panel panel-accent">
          <div className="panel-header">
            <h2>Diff And Feedback</h2>
            <span>{history.length} history events</span>
          </div>
          <div className="stack-list">
            {items.map((item) => (
              <article className="stack-item" key={`diff-${item.item_id}`}>
                <strong>Item #{item.item_id}</strong>
                <div className="stack-item__meta">
                  <span>added: {Number(item.diff.added ?? 0)}</span>
                  <span>updated: {Number(item.diff.updated ?? 0)}</span>
                  <span>removed: {Number(item.diff.removed ?? 0)}</span>
                </div>
                <button type="button">Request Rework</button>
              </article>
            ))}
          </div>
        </section>
      </div>

      <section className="panel">
        <div className="panel-header">
          <h2>Approval History</h2>
          <span>{history.length}</span>
        </div>
        <div className="stack-list">
          {history.map((entry, index) => (
            <article className="stack-item" key={`${entry.stage}-${entry.actor}-${index}`}>
              <strong>{entry.stage}</strong>
              <div className="stack-item__meta">
                <span>{entry.actor}</span>
                <span>{entry.action}</span>
                {entry.at ? <span>{entry.at}</span> : null}
              </div>
            </article>
          ))}
        </div>
      </section>
    </section>
  );
}
