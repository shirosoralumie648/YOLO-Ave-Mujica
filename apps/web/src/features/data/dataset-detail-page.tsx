import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getDatasetDetail, listDatasetItems, listDatasetSnapshots } from "./api";

export function DatasetDetailPage() {
  const { datasetId = "0" } = useParams();

  const detailQuery = useQuery({
    queryKey: ["dataset", datasetId],
    queryFn: () => getDatasetDetail(datasetId),
  });

  const itemsQuery = useQuery({
    queryKey: ["dataset-items", datasetId],
    queryFn: () => listDatasetItems(datasetId),
  });

  const snapshotsQuery = useQuery({
    queryKey: ["dataset-snapshots", datasetId],
    queryFn: () => listDatasetSnapshots(datasetId),
  });

  if (detailQuery.isError) {
    return (
      <section className="page-stack">
        <h1>Dataset Detail</h1>
        <p role="alert">{detailQuery.error.message}</p>
      </section>
    );
  }

  if (detailQuery.isLoading || itemsQuery.isLoading || snapshotsQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Dataset Detail</h1>
        <p>Loading dataset.</p>
      </section>
    );
  }

  if (!detailQuery.data || itemsQuery.isError || snapshotsQuery.isError || !itemsQuery.data || !snapshotsQuery.data) {
    const message =
      itemsQuery.error?.message ?? snapshotsQuery.error?.message ?? "Failed to load dataset dependencies.";
    return (
      <section className="page-stack">
        <h1>Dataset Detail</h1>
        <p role="alert">{message}</p>
      </section>
    );
  }

  const dataset = detailQuery.data;
  const previewItems = itemsQuery.data.items.slice(0, 6);

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Dataset {dataset.id}</p>
          <h1>{dataset.name}</h1>
          <p className="page-summary">
            {dataset.bucket}/{dataset.prefix}
          </p>
        </div>
      </header>

      <section className="panel">
        <div className="panel-header">
          <h2>Summary</h2>
          <span>{dataset.snapshot_count} snapshots</span>
        </div>
        <div className="data-meta">
          <span>{dataset.item_count} items</span>
          {dataset.latest_snapshot_id ? (
            <Link to={`/data/snapshots/${dataset.latest_snapshot_id}`}>
              {dataset.latest_snapshot_version || `Snapshot #${dataset.latest_snapshot_id}`}
            </Link>
          ) : (
            <span>No snapshots yet</span>
          )}
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Item preview</h2>
          <span>{itemsQuery.data.items.length} total</span>
        </div>
        {previewItems.length === 0 ? (
          <p>No items are available for this dataset.</p>
        ) : (
          <div className="stack-list">
            {previewItems.map((item) => (
              <article className="stack-item data-item-card" key={item.id || item.object_key}>
                <strong>{item.object_key}</strong>
                <span>{item.etag || "No etag"}</span>
              </article>
            ))}
          </div>
        )}
      </section>

      <section className="panel panel-accent">
        <div className="panel-header">
          <h2>Snapshots</h2>
          <span>{snapshotsQuery.data.items.length}</span>
        </div>
        {snapshotsQuery.data.items.length === 0 ? (
          <p>No snapshots yet</p>
        ) : (
          <div className="stack-list">
            {snapshotsQuery.data.items.map((snapshot) => (
              <article className="stack-item data-snapshot-card" key={snapshot.id}>
                <div className="data-snapshot-card__row">
                  <Link to={`/data/snapshots/${snapshot.id}`}>{snapshot.version}</Link>
                  {snapshot.based_on_snapshot_id ? (
                    <Link to={`/data/diff?before=${snapshot.based_on_snapshot_id}&after=${snapshot.id}`}>
                      Compare with previous
                    </Link>
                  ) : (
                    <span />
                  )}
                </div>
                <span>{snapshot.note || "No note"}</span>
              </article>
            ))}
          </div>
        )}
      </section>
    </section>
  );
}
