import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TaskOverviewPage } from "./task-overview-page";

vi.stubGlobal(
  "fetch",
  vi.fn(() =>
    Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({
          open_task_count: 4,
          blocked_task_count: 1,
          review_backlog_count: 6,
          failed_recent_jobs: 2,
          blockers: [
            {
              task_id: 9,
              title: "Schema mismatch review",
              reason: "schema mismatch",
              status: "blocked",
              minutes_idle: 180,
            },
          ],
          longest_idle_task: { id: 9, title: "Schema mismatch review", status: "blocked" },
        }),
    }),
  ) as any,
);

describe("TaskOverviewPage", () => {
  it("renders summary cards and blocker list", async () => {
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/projects/1/overview"]}>
          <Routes>
            <Route path="/projects/:projectId/overview" element={<TaskOverviewPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Open Tasks")).toBeInTheDocument();
    expect(await screen.findByText("Schema mismatch review")).toBeInTheDocument();
    expect(await screen.findByText(/Longest Idle Task/i)).toBeInTheDocument();
  });
});
