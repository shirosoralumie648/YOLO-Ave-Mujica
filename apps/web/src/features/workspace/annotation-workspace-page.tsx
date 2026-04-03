import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { startTransition, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  getAnnotationWorkspace,
  saveAnnotationWorkspaceDraft,
  submitAnnotationWorkspace,
  type AnnotationWorkspace,
} from "./api";

function toTitleCase(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

function stringifyDraftBody(workspace: AnnotationWorkspace | undefined) {
  return JSON.stringify(workspace?.draft.body ?? {}, null, 2);
}

function parseDraftBody(input: string) {
  const parsed = JSON.parse(input) as unknown;
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("Draft JSON must be an object.");
  }
  return parsed as Record<string, unknown>;
}

export function AnnotationWorkspacePage() {
  const { taskId = "" } = useParams();
  const queryClient = useQueryClient();
  const [draftText, setDraftText] = useState("{}");
  const [localError, setLocalError] = useState("");

  const workspaceQuery = useQuery({
    queryKey: ["annotation-workspace", taskId],
    queryFn: () => getAnnotationWorkspace(taskId),
  });

  useEffect(() => {
    if (!workspaceQuery.data) {
      return;
    }

    startTransition(() => {
      setDraftText(stringifyDraftBody(workspaceQuery.data));
      setLocalError("");
    });
  }, [workspaceQuery.data?.draft.revision, workspaceQuery.data?.task.status, workspaceQuery.data]);

  const saveDraftMutation = useMutation({
    mutationFn: async () => {
      const body = parseDraftBody(draftText);
      return saveAnnotationWorkspaceDraft(taskId, {
        actor: "annotator-1",
        base_revision: workspaceQuery.data?.draft.revision ?? 0,
        body,
      });
    },
    onSuccess: (workspace) => {
      queryClient.setQueryData(["annotation-workspace", taskId], workspace);
      setLocalError("");
    },
    onError: (error) => {
      setLocalError(error instanceof Error ? error.message : "Failed to save draft.");
    },
  });

  const submitMutation = useMutation({
    mutationFn: () => submitAnnotationWorkspace(taskId, "annotator-1"),
    onSuccess: async (workspace) => {
      queryClient.setQueryData(["annotation-workspace", taskId], workspace);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["task", taskId] }),
        queryClient.invalidateQueries({ queryKey: ["tasks"] }),
        queryClient.invalidateQueries({ queryKey: ["overview", 1] }),
      ]);
      setLocalError("");
    },
  });

  if (workspaceQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Annotation Workspace</h1>
        <p>Loading annotation workspace.</p>
      </section>
    );
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <section className="page-stack">
        <h1>Annotation Workspace</h1>
        <p role="alert">Failed to load annotation workspace.</p>
      </section>
    );
  }

  const workspace = workspaceQuery.data;
  const revision = workspace.draft.revision ?? 0;
  const statusLabel = toTitleCase(workspace.task.status || workspace.draft.state || "draft");

  return (
    <section className="page-stack workspace-shell">
      <header className="page-hero workspace-hero">
        <div>
          <p className="page-kicker">Task {workspace.task.id}</p>
          <h1>Annotation Workspace</h1>
          <p className="page-summary">
            Review asset context, adjust the draft payload, then persist or submit the task from one screen.
          </p>
        </div>
        <div className="hero-meter">
          <span>Status</span>
          <strong>{statusLabel}</strong>
          <small>Revision {revision}</small>
        </div>
      </header>

      <div className="workspace-grid">
        <aside className="panel workspace-tools">
          <div className="panel-header">
            <h2>Asset Context</h2>
            <span>{workspace.asset.snapshot_version || "snapshot pending"}</span>
          </div>
          <dl className="workspace-summary">
            <dt>Object key</dt>
            <dd>{workspace.asset.object_key}</dd>
            <dt>Dataset</dt>
            <dd>{workspace.asset.dataset_name || `Dataset ${workspace.asset.dataset_id ?? "n/a"}`}</dd>
            <dt>Snapshot</dt>
            <dd>{workspace.asset.snapshot_version || "Unversioned"}</dd>
            <dt>Media</dt>
            <dd>{toTitleCase(workspace.task.media_kind || "image")}</dd>
          </dl>
          <div className="action-row">
            <Link to={`/tasks/${workspace.task.id}`} className="button-secondary action-link">
              Back To Task
            </Link>
          </div>
        </aside>

        <main className="panel workspace-main">
          <div className="panel-header">
            <h2>Draft Payload</h2>
            <span>{workspace.draft.state || "draft"}</span>
          </div>
          <label className="workspace-editor">
            <span>Editable JSON</span>
            <textarea value={draftText} onChange={(event) => setDraftText(event.target.value)} spellCheck={false} />
          </label>
          {localError ? <p role="alert">{localError}</p> : null}
          <div className="panel-header">
            <h2>Rendered Preview</h2>
            <span>{workspace.asset.object_key}</span>
          </div>
          <pre className="workspace-preview">{draftText}</pre>
        </main>

        <aside className="panel panel-accent workspace-sidebar">
          <div className="panel-header">
            <h2>Session Actions</h2>
            <span>{statusLabel}</span>
          </div>
          <div className="action-row">
            <button type="button" disabled={saveDraftMutation.isPending} onClick={() => void saveDraftMutation.mutateAsync()}>
              Save Draft
            </button>
            <button
              type="button"
              className="button-secondary"
              disabled={submitMutation.isPending}
              onClick={() => void submitMutation.mutateAsync()}
            >
              Submit Task
            </button>
          </div>
          <p className="page-summary">
            Actor is fixed to <strong>annotator-1</strong> for this MVP shell. Later slices can replace this with session
            identity.
          </p>
          {(saveDraftMutation.isSuccess || submitMutation.isSuccess) && !localError ? (
            <p className="workspace-note">{statusLabel}</p>
          ) : null}
          {submitMutation.isError ? (
            <p role="alert">Failed to submit task: {submitMutation.error.message}</p>
          ) : null}
        </aside>
      </div>
    </section>
  );
}
