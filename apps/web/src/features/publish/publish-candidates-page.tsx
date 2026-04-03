import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createPublishBatch, listPublishCandidates, type SuggestedPublishGroup } from "./api";

function toReasonText(summary: Record<string, unknown>) {
  return String(summary.reason ?? summary.source_model ?? "rules-based suggestion");
}

export function PublishCandidatesPage() {
  const queryClient = useQueryClient();
  const candidatesQuery = useQuery({
    queryKey: ["publish-candidates", 1],
    queryFn: () => listPublishCandidates(1),
  });

  const createBatchMutation = useMutation({
    mutationFn: (group: SuggestedPublishGroup) =>
      createPublishBatch({
        project_id: 1,
        snapshot_id: group.snapshot_id,
        source: "suggested",
        rule_summary: group.summary,
        items: group.items.map((item) => ({
          candidate_id: item.candidate_id,
          task_id: item.task_id,
          dataset_id: item.dataset_id,
          snapshot_id: group.snapshot_id,
          item_payload: item.item_payload,
        })),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-candidates", 1] });
    },
  });

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Publish Candidates</h1>
          <p className="page-summary">
            Review grouped publish suggestions and promote them into a formal approval batch.
          </p>
        </div>
      </header>

      {candidatesQuery.isLoading ? <p>Loading publish candidates.</p> : null}
      {candidatesQuery.isError ? (
        <p role="alert">Failed to load publish candidates: {candidatesQuery.error.message}</p>
      ) : null}

      {candidatesQuery.data ? (
        candidatesQuery.data.items.length === 0 ? (
          <section className="panel panel-accent">
            <p>No suggested publish groups are available right now.</p>
          </section>
        ) : (
          <section className="panel">
            <div className="panel-header">
              <h2>Suggested groups</h2>
              <span>{candidatesQuery.data.items.length}</span>
            </div>
            <div className="stack-list">
              {candidatesQuery.data.items.map((group) => (
                <article className="stack-item" key={group.suggestion_key}>
                  <strong>{group.suggestion_key}</strong>
                  <div className="stack-item__meta">
                    <span>{toReasonText(group.summary)}</span>
                    <span>Snapshot {group.snapshot_id}</span>
                    <span>{group.items.length} candidate{group.items.length === 1 ? "" : "s"}</span>
                  </div>
                  <button
                    type="button"
                    onClick={() => void createBatchMutation.mutateAsync(group)}
                    disabled={createBatchMutation.isPending}
                  >
                    Create Publish Batch
                  </button>
                </article>
              ))}
            </div>
          </section>
        )
      ) : null}
    </section>
  );
}
