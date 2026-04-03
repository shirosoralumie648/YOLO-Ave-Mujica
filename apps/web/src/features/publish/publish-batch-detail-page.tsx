import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getPublishBatch, ownerApprove, reviewApprove } from "./api";

function toTitleCase(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

export function PublishBatchDetailPage() {
  const { batchId = "" } = useParams();
  const queryClient = useQueryClient();
  const batchQuery = useQuery({
    queryKey: ["publish-batch", batchId],
    queryFn: () => getPublishBatch(batchId),
  });

  const ownerApproveMutation = useMutation({
    mutationFn: () => ownerApprove(batchId, "owner-1"),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
    },
  });

  const reviewApproveMutation = useMutation({
    mutationFn: () => reviewApprove(batchId, "reviewer-1"),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
    },
  });

  if (batchQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Publish Batch</h1>
        <p>Loading publish batch.</p>
      </section>
    );
  }

  if (batchQuery.isError || !batchQuery.data) {
    return (
      <section className="page-stack">
        <h1>Publish Batch</h1>
        <p role="alert">Failed to load publish batch.</p>
      </section>
    );
  }

  const batch = batchQuery.data;
  const items = batch.items ?? [];
  const feedback = batch.feedback ?? [];

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Snapshot {batch.snapshot_id}</p>
          <h1>Publish Batch #{batch.id}</h1>
          <p className="page-summary">
            Track reviewer approval, owner approval, frozen item payloads, and structured feedback.
          </p>
        </div>
        <div className="hero-meter">
          <span>Status</span>
          <strong>{batch.status}</strong>
          <small>{items.length} items ready for approval</small>
        </div>
      </header>

      <section className="panel">
        <div className="panel-header">
          <h2>Batch items</h2>
          <Link className="panel-link" to={`/review/workspace/${batch.id}`}>
            Open Review Workspace
          </Link>
        </div>
        {items.length === 0 ? (
          <p>No frozen items are attached to this batch yet.</p>
        ) : (
          <div className="stack-list">
            {items.map((item) => (
              <article className="stack-item" key={item.id}>
                <strong>Candidate #{item.candidate_id}</strong>
                <div className="stack-item__meta">
                  <span>Task #{item.task_id}</span>
                  <span>Dataset #{item.dataset_id}</span>
                  <span>Snapshot #{item.snapshot_id}</span>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section className="panel panel-accent">
        <div className="panel-header">
          <h2>Structured feedback</h2>
          <span>{feedback.length}</span>
        </div>
        {feedback.length === 0 ? (
          <p>No structured feedback has been recorded yet.</p>
        ) : (
          <div className="stack-list">
            {feedback.map((entry) => (
              <article className="stack-item" key={entry.id}>
                <strong>
                  {toTitleCase(entry.scope)} · {toTitleCase(entry.stage)}
                </strong>
                <div className="stack-item__meta">
                  <span>{entry.reason_code}</span>
                  <span>{toTitleCase(entry.severity)}</span>
                  <span>{entry.comment || "No comment"}</span>
                </div>
              </article>
            ))}
          </div>
        )}
        <div className="action-row">
          <button type="button" onClick={() => void reviewApproveMutation.mutateAsync()}>
            Reviewer Approve
          </button>
          <button type="button" onClick={() => void ownerApproveMutation.mutateAsync()}>
            Owner Approve
          </button>
        </div>
      </section>
    </section>
  );
}
