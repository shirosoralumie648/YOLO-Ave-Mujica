import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { diffSnapshots } from "./api";

type ChangeType = "Added" | "Removed" | "Updated";

type ChangeItem = {
  type: ChangeType;
  id: number | string;
  categoryId?: number;
  iou?: number;
};

function parsePositiveInt(value: string | null) {
  if (!value || !/^\d+$/.test(value)) {
    return undefined;
  }
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed <= 0) {
    return undefined;
  }
  return parsed;
}

function toChange(
  type: ChangeType,
  item: { item_id?: number; annotation_id?: number; category_id?: number; iou?: number },
  index: number,
): ChangeItem {
  return {
    type,
    id: item.item_id ?? item.annotation_id ?? `unknown-${index + 1}`,
    categoryId: item.category_id,
    iou: item.iou,
  };
}

export function SnapshotDiffPage() {
  const [searchParams] = useSearchParams();

  const before = parsePositiveInt(searchParams.get("before"));
  const after = parsePositiveInt(searchParams.get("after"));
  const hasValidParams = before !== undefined && after !== undefined;

  const diffQuery = useQuery({
    queryKey: ["snapshot-diff", before, after],
    queryFn: () => diffSnapshots(before as number, after as number),
    enabled: hasValidParams,
  });

  if (!hasValidParams) {
    return (
      <section className="page-stack">
        <h1>Snapshot Diff</h1>
        <p role="alert">Both before and after snapshot ids are required.</p>
      </section>
    );
  }

  if (diffQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Snapshot Diff</h1>
        <p>Loading diff.</p>
      </section>
    );
  }

  if (diffQuery.isError || !diffQuery.data) {
    return (
      <section className="page-stack">
        <h1>Snapshot Diff</h1>
        <p role="alert">{diffQuery.error?.message || "Failed to load snapshot diff."}</p>
      </section>
    );
  }

  const diff = diffQuery.data;
  const addedCount = diff.stats.added ?? diff.stats.added_count ?? diff.adds.length;
  const removedCount = diff.stats.removed ?? diff.stats.removed_count ?? diff.removes.length;
  const updatedCount = diff.stats.updated ?? diff.stats.updated_count ?? diff.updates.length;
  const changes: ChangeItem[] = [
    ...diff.adds.map((item, index) => toChange("Added", item, index)),
    ...diff.removes.map((item, index) => toChange("Removed", item, index)),
    ...diff.updates.map((item, index) => toChange("Updated", item, index)),
  ];

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">
            Before #{before} to After #{after}
          </p>
          <h1>Snapshot Diff</h1>
        </div>
        <div className="hero-meter">
          <span>Compatibility</span>
          <strong>{diff.compatibility_score.toFixed(2)}</strong>
        </div>
      </header>

      <section className="diff-stats">
        <article className="panel summary-card">
          <span className="summary-card__title">Added</span>
          <strong className="summary-card__count">{addedCount}</strong>
        </article>
        <article className="panel summary-card">
          <span className="summary-card__title">Removed</span>
          <strong className="summary-card__count">{removedCount}</strong>
        </article>
        <article className="panel summary-card">
          <span className="summary-card__title">Updated</span>
          <strong className="summary-card__count">{updatedCount}</strong>
        </article>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Annotation Changes</h2>
          <span>{changes.length}</span>
        </div>
        {changes.length === 0 ? (
          <p>No annotation delta detected between these snapshots.</p>
        ) : (
          <div className="stack-list">
            {changes.map((change, index) => (
              <article className="stack-item" key={`${change.type}-${change.id}-${index}`}>
                <span>
                  {change.type} · item {change.id}
                  {change.categoryId !== undefined ? ` · category ${change.categoryId}` : ""}
                  {change.type === "Updated" && change.iou !== undefined ? ` · IOU ${change.iou.toFixed(2)}` : ""}
                </span>
              </article>
            ))}
          </div>
        )}
      </section>
    </section>
  );
}
