import { startTransition, useDeferredValue, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { createTask, listTasks, type CreateTaskPayload, type TaskItem } from "./api";

function toTitleCase(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

function formatRelativeTime(iso: string) {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffHours = Math.max(1, Math.round(diffMs / (1000 * 60 * 60)));
  if (diffHours < 24) {
    return `${diffHours}h ago`;
  }

  return `${Math.round(diffHours / 24)}d ago`;
}

function taskMeta(task: TaskItem) {
  return [
    toTitleCase(task.kind),
    toTitleCase(task.status),
    toTitleCase(task.priority),
    task.assignee || "Unassigned",
  ].join(" · ");
}

const defaultFormState: CreateTaskPayload = {
  title: "",
  assignee: "",
  kind: "annotation",
  priority: "normal",
};

export function TaskListPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [formState, setFormState] = useState<CreateTaskPayload>(defaultFormState);

  const status = searchParams.get("status") ?? "";
  const kind = searchParams.get("kind") ?? "";
  const assignee = searchParams.get("assignee") ?? "";
  const priority = searchParams.get("priority") ?? "";
  const deferredAssignee = useDeferredValue(assignee);

  const tasksQuery = useQuery({
    queryKey: ["tasks", 1, status, kind, deferredAssignee, priority],
    queryFn: () =>
      listTasks(1, {
        status: status || undefined,
        kind: kind || undefined,
        assignee: deferredAssignee || undefined,
        priority: priority || undefined,
      }),
  });

  const createTaskMutation = useMutation({
    mutationFn: (payload: CreateTaskPayload) => createTask(1, payload),
    onSuccess: async () => {
      setFormState(defaultFormState);
      await queryClient.invalidateQueries({
        queryKey: ["tasks", 1],
      });
    },
  });

  function updateFilter(name: string, value: string) {
    startTransition(() => {
      const next = new URLSearchParams(searchParams);
      if (value) {
        next.set(name, value);
      } else {
        next.delete(name);
      }
      setSearchParams(next);
    });
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await createTaskMutation.mutateAsync({
      title: formState.title.trim(),
      assignee: formState.assignee.trim(),
      kind: formState.kind,
      priority: formState.priority,
    });
  }

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Task List</h1>
          <p className="page-summary">
            Filter the queue in place and add the next work item without leaving the shell.
          </p>
        </div>
      </header>

      <div className="tasks-layout">
        <section className="panel">
          <div className="panel-header">
            <h2>Queue filters</h2>
            <span>{tasksQuery.data?.items.length ?? 0} visible</span>
          </div>
          <div className="filter-grid">
            <label>
              Status
              <select value={status} onChange={(event) => updateFilter("status", event.target.value)}>
                <option value="">all</option>
                <option value="queued">queued</option>
                <option value="ready">ready</option>
                <option value="in_progress">in_progress</option>
                <option value="blocked">blocked</option>
                <option value="done">done</option>
              </select>
            </label>

            <label>
              Kind
              <select value={kind} onChange={(event) => updateFilter("kind", event.target.value)}>
                <option value="">all</option>
                <option value="annotation">annotation</option>
                <option value="review">review</option>
                <option value="qa">qa</option>
                <option value="ops">ops</option>
              </select>
            </label>

            <label>
              Priority
              <select value={priority} onChange={(event) => updateFilter("priority", event.target.value)}>
                <option value="">all</option>
                <option value="low">low</option>
                <option value="normal">normal</option>
                <option value="high">high</option>
                <option value="critical">critical</option>
              </select>
            </label>

            <label>
              Assignee
              <input
                value={assignee}
                onChange={(event) => updateFilter("assignee", event.target.value)}
                placeholder="annotator-1"
              />
            </label>
          </div>
        </section>

        <section className="panel panel-accent">
          <div className="panel-header">
            <h2>Create task</h2>
            <span>Minimal flow</span>
          </div>
          <form className="task-form" onSubmit={handleSubmit}>
            <label>
              Task title
              <input
                required
                value={formState.title}
                onChange={(event) =>
                  setFormState((current) => ({
                    ...current,
                    title: event.target.value,
                  }))
                }
                placeholder="Label dock cameras"
              />
            </label>

            <label>
              Assignee
              <input
                value={formState.assignee}
                onChange={(event) =>
                  setFormState((current) => ({
                    ...current,
                    assignee: event.target.value,
                  }))
                }
                placeholder="annotator-7"
              />
            </label>

            <label>
              Kind
              <select
                value={formState.kind}
                onChange={(event) =>
                  setFormState((current) => ({
                    ...current,
                    kind: event.target.value,
                  }))
                }
              >
                <option value="annotation">annotation</option>
                <option value="review">review</option>
                <option value="qa">qa</option>
                <option value="ops">ops</option>
              </select>
            </label>

            <label>
              Priority
              <select
                value={formState.priority}
                onChange={(event) =>
                  setFormState((current) => ({
                    ...current,
                    priority: event.target.value,
                  }))
                }
              >
                <option value="low">low</option>
                <option value="normal">normal</option>
                <option value="high">high</option>
                <option value="critical">critical</option>
              </select>
            </label>

            <button type="submit" disabled={createTaskMutation.isPending}>
              Create Task
            </button>
            {createTaskMutation.isError ? (
              <p role="alert">Failed to create task: {createTaskMutation.error.message}</p>
            ) : null}
          </form>
        </section>
      </div>

      <section className="panel">
        <div className="panel-header">
          <h2>Live queue</h2>
          <span>Project 1</span>
        </div>
        {tasksQuery.isLoading ? <p>Loading tasks.</p> : null}
        {tasksQuery.isError ? <p role="alert">Failed to load tasks: {tasksQuery.error.message}</p> : null}

        {tasksQuery.data ? (
          tasksQuery.data.items.length > 0 ? (
            <div className="stack-list">
              {tasksQuery.data.items.map((task) => (
                <article className="stack-item task-card" key={task.id}>
                  <div className="task-card__heading">
                    <strong>
                      <Link to={`/tasks/${task.id}`}>{task.title}</Link>
                    </strong>
                    <span>{formatRelativeTime(task.last_activity_at)}</span>
                  </div>
                  <span>{taskMeta(task)}</span>
                  {task.blocker_reason ? <span>{task.blocker_reason}</span> : null}
                </article>
              ))}
            </div>
          ) : (
            <p>No tasks match the current filters.</p>
          )
        ) : null}
      </section>
    </section>
  );
}
