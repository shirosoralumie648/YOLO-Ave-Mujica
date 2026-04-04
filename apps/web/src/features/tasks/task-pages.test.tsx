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
  it("renders primary navigation links on the root-scoped shell route", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<AppShell />}>
            <Route index element={<div>Overview Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("Overview Page")).toBeInTheDocument();
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
  renderWithProviders("/tasks", <TaskListPage />, "/tasks");
  expect(await screen.findByText("Annotate loading-dock batch")).toBeInTheDocument();
  expect(await screen.findByText("Queued · High · annotator-1")).toBeInTheDocument();
  expect(await screen.findByText("No recent activity")).toBeInTheDocument();
});

it("renders task detail metadata", async () => {
  renderWithProviders("/tasks/1", <TaskDetailPage />, "/tasks/:taskId");
  expect(await screen.findByText(/annotator-1/i)).toBeInTheDocument();
  expect(await screen.findByText(/Unspecified task assigned to annotator-1/i)).toBeInTheDocument();
});
