import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { AppShell } from "../../app/layout/app-shell";
import { TaskListPage } from "./task-list-page";
import { TaskDetailPage } from "./task-detail-page";

describe("AppShell", () => {
  it("renders primary navigation links", () => {
    render(
      <MemoryRouter initialEntries={["/projects/1/overview"]}>
        <Routes>
          <Route path="/projects/:projectId" element={<AppShell />}>
            <Route path="overview" element={<div>Overview Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: /overview/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /tasks/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /data/i })).toBeInTheDocument();
  });
});

vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => {
  const url = String(input);
  if (url.endsWith("/v1/projects/1/tasks")) {
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { id: 1, title: "Annotate loading-dock batch", status: "queued", priority: "high", assignee: "annotator-1" }
        ],
      }),
    });
  }
  return Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ id: 1, title: "Annotate loading-dock batch", status: "queued", priority: "high", assignee: "annotator-1" }),
  });
}) as any);

function renderWithProviders(path: string, element: ReactNode, route: string) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path={route} element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it("renders task list rows", async () => {
  renderWithProviders("/projects/1/tasks", <TaskListPage />, "/projects/:projectId/tasks");
  expect(await screen.findByText("Annotate loading-dock batch")).toBeInTheDocument();
});

it("renders task detail metadata", async () => {
  renderWithProviders("/projects/1/tasks/1", <TaskDetailPage />, "/projects/:projectId/tasks/:taskId");
  expect(await screen.findByText(/annotator-1/i)).toBeInTheDocument();
});
