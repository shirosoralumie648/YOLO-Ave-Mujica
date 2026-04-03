import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewWorkspacePage } from "./review-workspace-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPage(initialEntry = "/review/workspace/71") {
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
          <Route path="/review/workspace/:batchId" element={<ReviewWorkspacePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ReviewWorkspacePage", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("renders overlay, diff stats, and item feedback controls", async () => {
    vi.mocked(global.fetch).mockResolvedValue(
      jsonResponse({
        batch: { id: 71, snapshot_id: 15, status: "owner_pending" },
        items: [
          {
            item_id: 801,
            candidate_id: 401,
            task_id: 51,
            overlay: { boxes: [{ label: "car", x: 0.1, y: 0.2, w: 0.3, h: 0.4 }] },
            diff: { added: 1, updated: 0, removed: 0 },
            feedback: [],
          },
        ],
        history: [{ stage: "review", actor: "reviewer-1", action: "approve" }],
      }),
    );

    renderPage();

    expect(await screen.findByRole("heading", { name: "Review Workspace" })).toBeInTheDocument();
    expect(await screen.findByText(/added: 1/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Request Rework/i })).toBeInTheDocument();
  });
});
