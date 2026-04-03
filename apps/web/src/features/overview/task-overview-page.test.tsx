import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskOverviewPage } from "./task-overview-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <TaskOverviewPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("TaskOverviewPage", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("loads overview cards, blockers, and failed jobs", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        summary_cards: [
          { id: "open-tasks", title: "Open Tasks", count: 6, href: "/tasks" },
          { id: "blocked-tasks", title: "Blocked Tasks", count: 2, href: "/tasks?status=blocked" },
        ],
        blockers: [
          {
            id: "task-1",
            title: "Blocked review batch",
            reason: "schema mismatch",
            href: "/tasks/1",
          },
        ],
        longest_idle_task: {
          id: 4,
          title: "Review lane 4",
          status: "blocked",
          priority: "high",
          assignee: "reviewer-2",
          last_activity_at: "2026-04-01T08:00:00Z",
        },
        recent_failed_jobs: [
          {
            id: 9,
            job_type: "zero-shot",
            status: "failed",
            error_msg: "provider unavailable",
          },
        ],
      }),
    );

    renderPage();

    expect(await screen.findByRole("heading", { name: "Task Overview" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /open tasks/i })).toBeInTheDocument();
    expect(screen.getByText("Blocked review batch")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /blocked review batch/i })).toHaveAttribute("href", "/tasks/1");
    expect(screen.getByText("schema mismatch")).toBeInTheDocument();
    expect(screen.getByText("Review lane 4")).toBeInTheDocument();
    expect(screen.getByText("zero-shot")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/projects/1/overview", expect.any(Object));
  });
});
