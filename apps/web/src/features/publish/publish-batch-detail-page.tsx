import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { addBatchFeedback, getPublishBatch, ownerApprove, reviewApprove, type CreateFeedbackPayload } from "./api";

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
  const [batchFeedback, setBatchFeedback] = useState<CreateFeedbackPayload>({
    stage: "owner",
    action: "rework",
    reason_code: "coverage_gap",
    severity: "high",
    influence_weight: 1,
    comment: "",
    actor: "owner-1",
  });
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

  const addBatchFeedbackMutation = useMutation({
    mutationFn: (payload: CreateFeedbackPayload) => addBatchFeedback(batchId, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
      await queryClient.invalidateQueries({ queryKey: ["publish-workspace", batchId] });
      setBatchFeedback((current) => ({ ...current, comment: "" }));
    },
  });

  async function handleBatchFeedbackSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await addBatchFeedbackMutation.mutateAsync(batchFeedback);
  }

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
        <form className="task-form" onSubmit={handleBatchFeedbackSubmit}>
          <label>
            Reason code
            <select
              value={batchFeedback.reason_code}
              onChange={(event) =>
                setBatchFeedback((current) => ({ ...current, reason_code: event.target.value }))
              }
            >
              <option value="coverage_gap">coverage_gap</option>
              <option value="trajectory_break">trajectory_break</option>
            </select>
          </label>

          <label>
            Severity
            <select
              value={batchFeedback.severity}
              onChange={(event) =>
                setBatchFeedback((current) => ({ ...current, severity: event.target.value }))
              }
            >
              <option value="high">high</option>
              <option value="critical">critical</option>
            </select>
          </label>

          <label>
            Influence weight
            <input
              type="number"
              min={0.1}
              step={0.1}
              value={batchFeedback.influence_weight}
              onChange={(event) =>
                setBatchFeedback((current) => ({
                  ...current,
                  influence_weight: Number(event.target.value),
                }))
              }
            />
          </label>

          <label>
            Comment
            <input
              value={batchFeedback.comment ?? ""}
              onChange={(event) =>
                setBatchFeedback((current) => ({ ...current, comment: event.target.value }))
              }
            />
          </label>

          <button type="submit" disabled={addBatchFeedbackMutation.isPending}>
            Submit Batch Feedback
          </button>
        </form>
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
