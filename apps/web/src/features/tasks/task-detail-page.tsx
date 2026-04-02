import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getTask, transitionTask } from "./api";

function toTitleCase(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

function formatRelativeTime(iso: string | undefined) {
  if (!iso) {
    return "No recent activity";
  }

  const diffMs = Date.now() - new Date(iso).getTime();
  const diffHours = Math.max(1, Math.round(diffMs / (1000 * 60 * 60)));
  if (diffHours < 24) {
    return `${diffHours}h ago`;
  }

  return `${Math.round(diffHours / 24)}d ago`;
}

type ActionDefinition = {
  label: string;
  status: string;
  requiresReason?: boolean;
  tone?: "primary" | "secondary";
};

function actionsForStatus(status: string): ActionDefinition[] {
  switch (status) {
    case "queued":
      return [{ label: "Mark Ready", status: "ready" }];
    case "ready":
      return [
        { label: "Start Task", status: "in_progress" },
        { label: "Mark Blocked", status: "blocked", requiresReason: true, tone: "secondary" },
      ];
    case "in_progress":
      return [
        { label: "Mark Done", status: "done" },
        { label: "Mark Blocked", status: "blocked", requiresReason: true, tone: "secondary" },
      ];
    case "blocked":
      return [
        { label: "Resume Task", status: "in_progress" },
        { label: "Move To Ready", status: "ready", tone: "secondary" },
      ];
    default:
      return [];
  }
}

export function TaskDetailPage() {
  const queryClient = useQueryClient();
  const { taskId = "0" } = useParams();
  const [blockerReason, setBlockerReason] = useState("");

  const taskQuery = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => getTask(taskId),
  });

  useEffect(() => {
    setBlockerReason(taskQuery.data?.blocker_reason ?? "");
  }, [taskQuery.data?.blocker_reason]);

  const transitionMutation = useMutation({
    mutationFn: ({ status, blockerReason: reason }: { status: string; blockerReason?: string }) =>
      transitionTask(taskId, {
        status,
        blocker_reason: reason,
      }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["task", taskId] }),
        queryClient.invalidateQueries({ queryKey: ["tasks"] }),
        queryClient.invalidateQueries({ queryKey: ["overview", 1] }),
      ]);
    },
  });

  if (taskQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Task Detail</h1>
        <p>Loading task.</p>
      </section>
    );
  }

  if (taskQuery.isError || !taskQuery.data) {
    return (
      <section className="page-stack">
        <h1>Task Detail</h1>
        <p role="alert">Failed to load task.</p>
      </section>
    );
  }

  const task = taskQuery.data;
  const actions = actionsForStatus(task.status);
  const needsBlockerReason = actions.some((action) => action.requiresReason);

  async function handleTransition(action: ActionDefinition) {
    await transitionMutation.mutateAsync({
      status: action.status,
      blockerReason: action.requiresReason ? blockerReason.trim() : undefined,
    });
  }

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Task {task.id}</p>
          <h1>{task.title}</h1>
          <p className="page-summary">
            {toTitleCase(task.kind)} task assigned to {task.assignee || "unassigned"}.
          </p>
        </div>
        <div className="hero-meter">
          <span>Status</span>
          <strong>{toTitleCase(task.status)}</strong>
          <small>{formatRelativeTime(task.last_activity_at)}</small>
        </div>
      </header>

      <div className="detail-grid">
        <section className="panel detail-meta">
          <div className="panel-header">
            <h2>Task metadata</h2>
            <span>{toTitleCase(task.priority)}</span>
          </div>
          <dl>
            <dt>Assignee</dt>
            <dd>{task.assignee || "Unassigned"}</dd>
            <dt>Dataset</dt>
            <dd>
              {task.dataset_id && task.dataset_name ? (
                <Link to={`/data/datasets/${task.dataset_id}`} className="inline-link">
                  {task.dataset_name}
                </Link>
              ) : (
                task.dataset_name || "No dataset context"
              )}
            </dd>
            <dt>Snapshot</dt>
            <dd>
              {task.snapshot_id && task.snapshot_version ? (
                <Link to={`/data/snapshots/${task.snapshot_id}`} className="inline-link">
                  {task.snapshot_version}
                </Link>
              ) : (
                task.snapshot_version || "No snapshot bound"
              )}
            </dd>
            <dt>Updated</dt>
            <dd>{formatRelativeTime(task.updated_at)}</dd>
          </dl>
        </section>

        <section className="panel panel-accent detail-actions">
          <div className="panel-header">
            <h2>Task actions</h2>
            <span>{actions.length} available</span>
          </div>
          {needsBlockerReason ? (
            <label className="detail-field">
              Blocker reason
              <input
                value={blockerReason}
                onChange={(event) => setBlockerReason(event.target.value)}
                placeholder="waiting for schema update"
              />
            </label>
          ) : null}
          <div className="action-row">
            {actions.length === 0 ? <p>No further actions available.</p> : null}
            {actions.map((action) => {
              const disabled =
                transitionMutation.isPending || (action.requiresReason ? blockerReason.trim() === "" : false);
              return (
                <button
                  key={action.label}
                  className={action.tone === "secondary" ? "button-secondary" : undefined}
                  type="button"
                  disabled={disabled}
                  onClick={() => void handleTransition(action)}
                >
                  {action.label}
                </button>
              );
            })}
          </div>
          {transitionMutation.isError ? (
            <p role="alert">Failed to transition task: {transitionMutation.error.message}</p>
          ) : null}
        </section>
      </div>

      {task.blocker_reason ? (
        <section className="panel">
          <div className="panel-header">
            <h2>Blocker</h2>
            <span>Needs action</span>
          </div>
          <p>{task.blocker_reason}</p>
        </section>
      ) : null}
    </section>
  );
}
