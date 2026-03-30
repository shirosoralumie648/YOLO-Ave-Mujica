import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";

import { AppShell } from "../../app/layout/app-shell";
import { TaskOverviewPage } from "./task-overview-page";
import { TaskListPage } from "../tasks/task-list-page";
import { TaskDetailPage } from "../tasks/task-detail-page";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("Task-first pages", () => {
  it("renders overview blockers and longest idle task from the API", async () => {
    stubFetch({
      "/v1/projects/1/overview": {
        project_id: 1,
        open_task_count: 7,
        review_backlog: 4,
        failed_recent_jobs: 1,
        longest_idle_task: {
          id: 11,
          title: "Oldest review handoff",
          assignee: "reviewer-1",
          status: "ready",
          last_activity_at: "2026-03-27T12:00:00Z",
        },
        blockers: [
          {
            kind: "review_backlog",
            severity: "high",
            title: "Review queue backlog",
            description: "4 items are waiting for review",
            href: "/review?project_id=1",
          },
          {
            kind: "failed_jobs",
            severity: "high",
            title: "Recent failed training or processing jobs",
            description: "1 recent jobs failed in the last 24 hours",
            href: "/training/runs?project_id=1&status=failed",
          },
        ],
      },
    });

    renderPage("/projects/1/overview?snapshot_id=snap-009");

    expect(await screen.findByText("Review queue backlog")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Task Overview" })).toBeInTheDocument();
    expect(screen.getByText("7", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByText("Oldest review handoff")).toBeInTheDocument();
    const queueLinks = screen.getAllByRole("link", { name: /Open task queue/i });
    expect(queueLinks).toHaveLength(2);
    for (const link of queueLinks) {
      expect(link).toHaveAttribute("href", "/projects/1/tasks?snapshot_id=snap-009");
    }
  });

  it("renders the project task list with preserved snapshot context", async () => {
    stubFetch({
      "/v1/projects/1/tasks": {
        items: [
          {
            id: 11,
            project_id: 1,
            dataset_id: 5,
            snapshot_id: 9,
            title: "Oldest review handoff",
            description: "Triaging backlog",
            assignee: "reviewer-1",
            status: "ready",
            priority: "high",
            last_activity_at: "2026-03-27T12:00:00Z",
            created_at: "2026-03-26T08:00:00Z",
            updated_at: "2026-03-27T12:00:00Z",
          },
        ],
      },
    });

    renderPage("/projects/1/tasks?snapshot_id=snap-009");

    expect(await screen.findByText("Oldest review handoff")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Task Queue" })).toBeInTheDocument();
    expect(screen.getByText("reviewer-1")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Open task detail/i })).toHaveAttribute(
      "href",
      "/projects/1/tasks/11?snapshot_id=snap-009",
    );
  });

  it("renders task detail with lineage context and a back link", async () => {
    stubFetch({
      "/v1/tasks/11": {
        id: 11,
        project_id: 1,
        dataset_id: 5,
        snapshot_id: 9,
        title: "Oldest review handoff",
        description: "Triaging backlog",
        assignee: "reviewer-1",
        status: "ready",
        priority: "high",
        last_activity_at: "2026-03-27T12:00:00Z",
        created_at: "2026-03-26T08:00:00Z",
        updated_at: "2026-03-27T12:00:00Z",
      },
    });

    renderPage("/projects/1/tasks/11?snapshot_id=snap-009");

    expect(await screen.findByRole("heading", { name: "Oldest review handoff" })).toBeInTheDocument();
    expect(screen.getByText("Dataset 5")).toBeInTheDocument();
    expect(screen.getByText("Snapshot 9")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Back to task queue/i })).toHaveAttribute(
      "href",
      "/projects/1/tasks?snapshot_id=snap-009",
    );
  });
});

function renderPage(initialEntry: string) {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        future={{ v7_relativeSplatPath: true, v7_startTransition: true }}
        initialEntries={[initialEntry]}
      >
        <Routes>
          <Route path="/projects/:projectId" element={<AppShell />}>
            <Route path="overview" element={<TaskOverviewPage />} />
            <Route path="tasks" element={<TaskListPage />} />
            <Route path="tasks/:taskId" element={<TaskDetailPage />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function stubFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      const parsed = new URL(url, "http://localhost");
      const payload = routes[parsed.pathname];

      if (payload === undefined) {
        return new Response(JSON.stringify({ error: `unhandled ${parsed.pathname}` }), {
          status: 404,
          headers: { "Content-Type": "application/json" },
        });
      }

      return new Response(JSON.stringify(payload), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}
