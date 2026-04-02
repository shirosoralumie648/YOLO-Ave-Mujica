import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "../../app/layout/app-shell";
import { DatasetDetailPage } from "./dataset-detail-page";
import { DatasetListPage } from "./dataset-list-page";
import { SnapshotDetailPage } from "./snapshot-detail-page";
import { SnapshotDiffPage } from "./snapshot-diff-page";

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
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/" element={<AppShell />}>
            <Route path="data" element={<DatasetListPage />} />
            <Route path="data/datasets/:datasetId" element={<DatasetDetailPage />} />
            <Route path="data/snapshots/:snapshotId" element={<SnapshotDetailPage />} />
            <Route path="data/diff" element={<SnapshotDiffPage />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Data pages", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("renders data nav and dataset list summary from /v1/datasets", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        items: [
          {
            id: 5,
            name: "dock-night",
            bucket: "platform-dev",
            prefix: "datasets/dock-night",
            item_count: 42,
            snapshot_count: 2,
            latest_snapshot_id: 12,
            latest_snapshot_version: "v3",
          },
        ],
      }),
    );

    renderPage("/data");

    const nav = screen.getByRole("navigation", { name: "Primary" });
    expect(within(nav).getByRole("link", { name: "Overview" })).not.toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(nav).getByRole("link", { name: "Data" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Data" })).toHaveAttribute("href", "/data");
    expect(await screen.findByRole("heading", { name: "Dataset List" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "dock-night" })).toHaveAttribute("href", "/data/datasets/5");
    expect(await screen.findByRole("link", { name: "v3" })).toHaveAttribute(
      "href",
      "/data/snapshots/12",
    );
    expect(screen.getByText("42 items")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/datasets", expect.any(Object));
  });

  it("renders empty state when there are no datasets", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(jsonResponse({ items: [] }));

    renderPage("/data");

    expect(await screen.findByText("No datasets are registered yet.")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/datasets", expect.any(Object));
  });

  it("renders dataset detail with snapshot and compare links", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({
          id: 5,
          name: "dock-night",
          bucket: "platform-dev",
          prefix: "datasets/dock-night",
          item_count: 42,
          snapshot_count: 2,
          latest_snapshot_id: 12,
          latest_snapshot_version: "v3",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          items: [
            {
              object_key: "train/night/a.jpg",
              etag: "abc123",
            },
          ],
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          items: [
            {
              id: 12,
              dataset_id: 5,
              version: "v2",
              based_on_snapshot_id: 11,
              note: "added new captures",
            },
          ],
        }),
      );

    renderPage("/data/datasets/5");

    expect(await screen.findByRole("heading", { name: "dock-night" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "v3" })).toHaveAttribute("href", "/data/snapshots/12");
    expect(screen.getByRole("link", { name: "v2" })).toHaveAttribute("href", "/data/snapshots/12");
    expect(screen.getByRole("link", { name: "Compare with previous" })).toHaveAttribute(
      "href",
      "/data/diff?before=11&after=12",
    );
    expect(screen.getByText("train/night/a.jpg")).toBeInTheDocument();

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/v1/datasets/5");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/v1/datasets/5/items");
    expect(fetchMock.mock.calls[2]?.[0]).toBe("/v1/datasets/5/snapshots");
  });

  it("renders api not-found text when dataset detail request fails", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ error: "dataset 999 not found" }, 404))
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse({ items: [] }));

    renderPage("/data/datasets/999");

    expect(await screen.findByRole("alert")).toHaveTextContent("dataset 999 not found");
  });

  it("renders detail api error immediately without waiting for secondary requests", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockImplementation((input) => {
      if (input === "/v1/datasets/999") {
        return Promise.resolve(jsonResponse({ error: "dataset 999 not found" }, 404));
      }
      return new Promise<Response>(() => {});
    });

    renderPage("/data/datasets/999");

    expect(await screen.findByRole("alert")).toHaveTextContent("dataset 999 not found");
  });

  it("renders snapshot detail metadata and compare link", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        id: 12,
        dataset_id: 5,
        dataset_name: "dock-night",
        project_id: 1,
        version: "v2",
        based_on_snapshot_id: 11,
        note: "qa relabel",
        annotation_count: 17,
      }),
    );

    renderPage("/data/snapshots/12");

    expect(await screen.findByRole("heading", { name: "v2" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "dock-night" })).toHaveAttribute("href", "/data/datasets/5");
    expect(screen.getByRole("link", { name: "Compare with previous" })).toHaveAttribute(
      "href",
      "/data/diff?before=11&after=12",
    );
    expect(screen.getByText("17")).toBeInTheDocument();
    expect(screen.getByText(/Parent:/i)).toHaveTextContent("#11");
    expect(screen.getByText("qa relabel")).toBeInTheDocument();
    expect(
      screen.getByText(/Publish status is not wired in this slice yet/i),
    ).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/snapshots/12", expect.any(Object));
  });

  it("renders snapshot not-found api text", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(jsonResponse({ error: "snapshot 999 not found" }, 404));

    renderPage("/data/snapshots/999");

    expect(await screen.findByRole("alert")).toHaveTextContent("snapshot 999 not found");
    expect(fetchMock).toHaveBeenCalledWith("/v1/snapshots/999", expect.any(Object));
  });

  it("renders snapshot diff parameter error when ids are missing", async () => {
    const fetchMock = vi.mocked(global.fetch);
    renderPage("/data/diff");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Both before and after snapshot ids are required.",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("renders snapshot diff parameter error when ids are malformed or non-positive", async () => {
    const fetchMock = vi.mocked(global.fetch);

    renderPage("/data/diff?before=abc&after=12");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Both before and after snapshot ids are required.",
    );
    expect(fetchMock).not.toHaveBeenCalled();

    cleanup();
    renderPage("/data/diff?before=0&after=12");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Both before and after snapshot ids are required.",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("renders snapshot diff stats and changes", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        adds: [{ item_id: 101, category_id: 8 }],
        removes: [{ item_id: 77, category_id: 2 }],
        updates: [{ item_id: 55, category_id: 4, iou: 0.63 }],
        stats: {
          added: 1,
          removed: 1,
          updated: 1,
        },
        compatibility_score: 0.91,
      }),
    );

    renderPage("/data/diff?before=11&after=12");

    expect(await screen.findByRole("heading", { name: "Snapshot Diff" })).toBeInTheDocument();
    expect(await screen.findByText("0.91")).toBeInTheDocument();
    expect(await screen.findByText("Added · item 101 · category 8")).toBeInTheDocument();
    expect(await screen.findByText("Removed · item 77 · category 2")).toBeInTheDocument();
    expect(await screen.findByText("Updated · item 55 · category 4 · IOU 0.63")).toBeInTheDocument();
    const addedCard = screen.getByText("Added").closest("article");
    const removedCard = screen.getByText("Removed").closest("article");
    const updatedCard = screen.getByText("Updated").closest("article");
    expect(addedCard).not.toBeNull();
    expect(removedCard).not.toBeNull();
    expect(updatedCard).not.toBeNull();
    expect(within(addedCard as HTMLElement).getByText("1")).toBeInTheDocument();
    expect(within(removedCard as HTMLElement).getByText("1")).toBeInTheDocument();
    expect(within(updatedCard as HTMLElement).getByText("1")).toBeInTheDocument();

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/snapshots/diff",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ before_snapshot_id: 11, after_snapshot_id: 12 }),
      }),
    );
  });

  it("renders empty diff state when there are no changes", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        adds: [],
        removes: [],
        updates: [],
        stats: {
          added: 0,
          removed: 0,
          updated: 0,
        },
        compatibility_score: 1,
      }),
    );

    renderPage("/data/diff?before=11&after=12");

    expect(
      await screen.findByText("No annotation delta detected between these snapshots."),
    ).toBeInTheDocument();
  });
});
