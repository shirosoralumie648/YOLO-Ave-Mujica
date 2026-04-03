import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { appRoutes } from "../../app/router";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function toPath(input: RequestInfo | URL) {
  if (typeof input === "string") {
    return input.startsWith("http") ? new URL(input).pathname : input;
  }
  if (input instanceof URL) {
    return input.pathname;
  }
  return new URL(input.url).pathname;
}

function renderApp(initialEntry = "/tasks/18/workspace") {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  const router = createMemoryRouter(appRoutes, {
    initialEntries: [initialEntry],
  });

  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe("AnnotationWorkspacePage", () => {
  beforeEach(() => {
    global.fetch = vi.fn((input, init) => {
      const path = toPath(input);
      const method = (init?.method ?? "GET").toUpperCase();

      if (method === "GET" && path === "/v1/tasks/18/workspace") {
        return Promise.resolve(
          jsonResponse({
            task: {
              id: 18,
              status: "in_progress",
              kind: "annotation",
              asset_object_key: "train/images/a.jpg",
              media_kind: "image",
            },
            asset: {
              dataset_id: 1,
              dataset_name: "yard-ops",
              object_key: "train/images/a.jpg",
              snapshot_version: "v7",
            },
            draft: {
              id: 31,
              revision: 2,
              state: "draft",
              body: { objects: [{ id: "box-1", label: "person" }] },
            },
          }),
        );
      }

      if (method === "PUT" && path === "/v1/tasks/18/workspace/draft") {
        return Promise.resolve(
          jsonResponse({
            task: {
              id: 18,
              status: "in_progress",
              kind: "annotation",
              asset_object_key: "train/images/a.jpg",
              media_kind: "image",
            },
            asset: {
              dataset_id: 1,
              dataset_name: "yard-ops",
              object_key: "train/images/a.jpg",
              snapshot_version: "v7",
            },
            draft: {
              id: 31,
              revision: 3,
              state: "draft",
              body: { objects: [{ id: "box-1", label: "person" }] },
            },
          }),
        );
      }

      if (method === "POST" && path === "/v1/tasks/18/workspace/submit") {
        return Promise.resolve(
          jsonResponse({
            task: {
              id: 18,
              status: "submitted",
              kind: "annotation",
              asset_object_key: "train/images/a.jpg",
              media_kind: "image",
            },
            asset: {
              dataset_id: 1,
              dataset_name: "yard-ops",
              object_key: "train/images/a.jpg",
              snapshot_version: "v7",
            },
            draft: {
              id: 31,
              revision: 4,
              state: "submitted",
              body: { objects: [{ id: "box-1", label: "person" }] },
            },
          }),
        );
      }

      return Promise.reject(new Error(`Unhandled request: ${method} ${path}`));
    }) as typeof global.fetch;
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("loads workspace data, saves draft, and submits the task", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.mocked(global.fetch);

    renderApp();

    expect(await screen.findByRole("button", { name: "Save Draft" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save Draft" }));
    await user.click(screen.getByRole("button", { name: "Submit Task" }));

    expect((await screen.findAllByText("Submitted")).length).toBeGreaterThan(0);
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/v1/tasks/18/workspace/draft", expect.any(Object)),
    );
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/v1/tasks/18/workspace/submit", expect.any(Object)),
    );
  });
});
