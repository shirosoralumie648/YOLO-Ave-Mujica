import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PublishBatchDetailPage } from "./publish-batch-detail-page";
import { PublishCandidatesPage } from "./publish-candidates-page";
import { ReviewQueuePage } from "../review/review-queue-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

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
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/review" element={<ReviewQueuePage />} />
          <Route path="/publish/candidates" element={<PublishCandidatesPage />} />
          <Route path="/publish/batches/:batchId" element={<PublishBatchDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Publish review pages", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("renders review queue with publish links", async () => {
    vi.mocked(global.fetch).mockResolvedValueOnce(
      jsonResponse({
        items: [
          { id: 71, title: "Publish lane 4", status: "owner_pending", snapshot_id: 15, kind: "review" },
        ],
      }),
    );

    renderPage("/review");

    expect(await screen.findByRole("heading", { name: "Review Queue" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /Publish lane 4/i })).toHaveAttribute(
      "href",
      "/publish/batches/71",
    );
  });

  it("renders suggested publish candidates and create-batch controls", async () => {
    vi.mocked(global.fetch).mockResolvedValueOnce(
      jsonResponse({
        items: [
          {
            snapshot_id: 15,
            suggestion_key: "risk-high-window-1",
            summary: { reason: "same-risk-window" },
            items: [
              {
                candidate_id: 401,
                task_id: 51,
                dataset_id: 9,
                item_payload: { task: { id: 51 }, snapshot: { id: 15, version: "v5" } },
              },
            ],
          },
        ],
      }),
    );

    renderPage("/publish/candidates");

    expect(await screen.findByRole("heading", { name: "Publish Candidates" })).toBeInTheDocument();
    expect(await screen.findByText(/same-risk-window/i)).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /Create Publish Batch/i })).toBeInTheDocument();
  });

  it("renders publish batch detail with feedback and owner actions", async () => {
    vi.mocked(global.fetch).mockResolvedValueOnce(
      jsonResponse({
        id: 71,
        snapshot_id: 15,
        project_id: 1,
        status: "owner_pending",
        source: "suggested",
        rule_summary: {},
        items: [
          { id: 801, candidate_id: 401, task_id: 51, dataset_id: 9, snapshot_id: 15, item_payload: {} },
        ],
        feedback: [
          {
            id: 1,
            scope: "batch",
            stage: "review",
            action: "comment",
            reason_code: "ready_for_publish",
            severity: "low",
            influence_weight: 1,
            comment: "",
          },
        ],
      }),
    );

    renderPage("/publish/batches/71");

    expect(await screen.findByRole("heading", { name: "Publish Batch #71" })).toBeInTheDocument();
    expect(screen.getByText(/owner_pending/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Owner Approve/i })).toBeInTheDocument();
  });
});
