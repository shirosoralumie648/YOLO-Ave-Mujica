import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { listDatasets } from "./api";

export function DatasetListPage() {
  const datasetsQuery = useQuery({
    queryKey: ["datasets", 1],
    queryFn: listDatasets,
  });

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Dataset List</h1>
          <p className="page-summary">Browse dataset coverage and jump directly into snapshots.</p>
        </div>
      </header>

      {datasetsQuery.isLoading ? <p>Loading datasets.</p> : null}
      {datasetsQuery.isError ? <p role="alert">Failed to load datasets: {datasetsQuery.error.message}</p> : null}

      {datasetsQuery.data ? (
        datasetsQuery.data.items.length === 0 ? (
          <section className="panel panel-accent">
            <p>No datasets are registered yet.</p>
          </section>
        ) : (
          <section className="panel">
            <div className="panel-header">
              <h2>Registered datasets</h2>
              <span>{datasetsQuery.data.items.length}</span>
            </div>
            <div className="stack-list">
              {datasetsQuery.data.items.map((dataset) => (
                <article className="stack-item data-card" key={dataset.id}>
                  <div className="data-card__heading">
                    <strong>
                      <Link to={`/data/datasets/${dataset.id}`}>{dataset.name}</Link>
                    </strong>
                    <span>
                      {dataset.snapshot_count} snapshot{dataset.snapshot_count === 1 ? "" : "s"}
                    </span>
                  </div>
                  <span>
                    {dataset.bucket}/{dataset.prefix}
                  </span>
                  <span>{dataset.item_count} items</span>
                  {dataset.latest_snapshot_id ? (
                    <Link to={`/data/snapshots/${dataset.latest_snapshot_id}`}>
                      {dataset.latest_snapshot_version || `Snapshot #${dataset.latest_snapshot_id}`}
                    </Link>
                  ) : (
                    <span>No snapshots yet</span>
                  )}
                </article>
              ))}
            </div>
          </section>
        )
      ) : null}
    </section>
  );
}
