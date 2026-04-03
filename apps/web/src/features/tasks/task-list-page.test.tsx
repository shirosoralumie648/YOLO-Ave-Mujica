import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskListPage } from "./task-list-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPage(initialEntry = "/tasks") {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/tasks" element={<TaskListPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("TaskListPage", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("loads tasks from URL filters", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        items: [
          {
            id: 1,
            project_id: 1,
            title: "Blocked review batch",
            kind: "review",
            status: "blocked",
            priority: "high",
            assignee: "reviewer-1",
            blocker_reason: "schema mismatch",
            last_activity_at: "2026-04-01T08:00:00Z",
            created_at: "2026-04-01T08:00:00Z",
            updated_at: "2026-04-01T08:00:00Z",
          },
        ],
      }),
    );

    renderPage("/tasks?status=blocked");

    expect(await screen.findByRole("heading", { name: "Task List" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("blocked")).toBeInTheDocument();
    expect(await screen.findByText("Blocked review batch")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Blocked review batch" })).toHaveAttribute("href", "/tasks/1");
    expect(fetchMock).toHaveBeenCalledWith("/v1/projects/1/tasks?status=blocked", expect.any(Object));

    const queueFiltersHeading = screen.getByRole("heading", { name: "Queue filters" });
    const queueFiltersPanel = queueFiltersHeading.closest("section");
    if (!queueFiltersPanel) {
      throw new Error("queue filters panel not found");
    }

    const statusSelect = within(queueFiltersPanel).getByLabelText("Status");
    expect(within(statusSelect).queryByRole("option", { name: "done" })).not.toBeInTheDocument();
    expect(within(statusSelect).getByRole("option", { name: "submitted" })).toBeInTheDocument();
    expect(within(statusSelect).getByRole("option", { name: "reviewing" })).toBeInTheDocument();
    expect(within(statusSelect).getByRole("option", { name: "rework_required" })).toBeInTheDocument();
    expect(within(statusSelect).getByRole("option", { name: "accepted" })).toBeInTheDocument();
    expect(within(statusSelect).getByRole("option", { name: "published" })).toBeInTheDocument();
    expect(within(statusSelect).getByRole("option", { name: "closed" })).toBeInTheDocument();

    const kindSelect = within(queueFiltersPanel).getByLabelText("Kind");
    expect(within(kindSelect).getByRole("option", { name: "training_candidate" })).toBeInTheDocument();
    expect(within(kindSelect).getByRole("option", { name: "promotion_review" })).toBeInTheDocument();
  });

  it("creates a task and refreshes the list", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(
        jsonResponse(
          {
            id: 2,
            project_id: 1,
            title: "Label dock cameras",
            kind: "review",
            status: "queued",
            priority: "normal",
            assignee: "annotator-7",
            blocker_reason: "",
            last_activity_at: "2026-04-01T08:00:00Z",
            created_at: "2026-04-01T08:00:00Z",
            updated_at: "2026-04-01T08:00:00Z",
          },
          201,
        ),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          items: [
            {
              id: 2,
              project_id: 1,
              title: "Label dock cameras",
              kind: "review",
              status: "queued",
              priority: "normal",
              assignee: "annotator-7",
              blocker_reason: "",
              last_activity_at: "2026-04-01T08:00:00Z",
              created_at: "2026-04-01T08:00:00Z",
              updated_at: "2026-04-01T08:00:00Z",
            },
          ],
        }),
      );

    const user = userEvent.setup();
    renderPage();

    const form = screen.getByRole("button", { name: "Create Task" }).closest("form");
    if (!form) {
      throw new Error("task creation form not found");
    }
    const createKindSelect = within(form).getByLabelText("Kind");
    expect(within(createKindSelect).queryByRole("option", { name: "annotation" })).not.toBeInTheDocument();
    expect(within(createKindSelect).getByRole("option", { name: "review" })).toBeInTheDocument();

    await user.type(within(form).getByLabelText("Task title"), "Label dock cameras");
    await user.type(within(form).getByLabelText("Assignee"), "annotator-7");
    await user.click(within(form).getByRole("button", { name: "Create Task" }));

    expect(await screen.findByText("Label dock cameras")).toBeInTheDocument();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));

    const [, createCall] = fetchMock.mock.calls;
    expect(createCall?.[0]).toBe("/v1/projects/1/tasks");
    expect(createCall?.[1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        title: "Label dock cameras",
        assignee: "annotator-7",
        kind: "review",
        priority: "normal",
      }),
    });
  });
});
