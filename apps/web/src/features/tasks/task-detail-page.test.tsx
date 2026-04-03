import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskDetailPage } from "./task-detail-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPage(initialEntry = "/tasks/4") {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/tasks/:taskId" element={<TaskDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("TaskDetailPage", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("loads task metadata and snapshot context", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        id: 4,
        project_id: 1,
        snapshot_id: 12,
        snapshot_version: "v7",
        dataset_id: 5,
        dataset_name: "dock-night",
        title: "Review lane 4",
        kind: "review",
        status: "blocked",
        priority: "high",
        assignee: "reviewer-2",
        blocker_reason: "waiting for schema update",
        last_activity_at: "2026-04-01T08:00:00Z",
        created_at: "2026-04-01T08:00:00Z",
        updated_at: "2026-04-01T08:00:00Z",
      }),
    );

    renderPage();

    expect(await screen.findByRole("heading", { name: "Review lane 4" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "dock-night" })).toHaveAttribute("href", "/data/datasets/5");
    expect(screen.getByRole("link", { name: "v7" })).toHaveAttribute("href", "/data/snapshots/12");
    expect(screen.getByText(/waiting for schema update/i)).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/tasks/4", expect.any(Object));
  });

  it("transitions a blocked task back to in progress", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({
          id: 4,
          project_id: 1,
          title: "Review lane 4",
          kind: "review",
          status: "blocked",
          priority: "high",
          assignee: "reviewer-2",
          blocker_reason: "waiting for schema update",
          last_activity_at: "2026-04-01T08:00:00Z",
          created_at: "2026-04-01T08:00:00Z",
          updated_at: "2026-04-01T08:00:00Z",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          id: 4,
          project_id: 1,
          title: "Review lane 4",
          kind: "review",
          status: "in_progress",
          priority: "high",
          assignee: "reviewer-2",
          blocker_reason: "",
          last_activity_at: "2026-04-02T08:00:00Z",
          created_at: "2026-04-01T08:00:00Z",
          updated_at: "2026-04-02T08:00:00Z",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          id: 4,
          project_id: 1,
          title: "Review lane 4",
          kind: "review",
          status: "in_progress",
          priority: "high",
          assignee: "reviewer-2",
          blocker_reason: "",
          last_activity_at: "2026-04-02T08:00:00Z",
          created_at: "2026-04-01T08:00:00Z",
          updated_at: "2026-04-02T08:00:00Z",
        }),
      );

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Resume Task" }));

    expect(await screen.findByText(/in progress/i)).toBeInTheDocument();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/v1/tasks/4/transition");
  });

  it("shows an annotation workspace launch link for annotation tasks", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        id: 4,
        project_id: 1,
        snapshot_id: 12,
        snapshot_version: "v7",
        dataset_id: 5,
        dataset_name: "dock-night",
        title: "Annotate lane 4",
        kind: "annotation",
        status: "in_progress",
        priority: "high",
        assignee: "annotator-2",
        blocker_reason: "",
        asset_object_key: "train/images/lane-4.jpg",
        media_kind: "image",
        last_activity_at: "2026-04-01T08:00:00Z",
        created_at: "2026-04-01T08:00:00Z",
        updated_at: "2026-04-01T08:00:00Z",
      }),
    );

    renderPage();

    expect(await screen.findByRole("link", { name: "Open Workspace" })).toHaveAttribute("href", "/tasks/4/workspace");
  });
});
