import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { getSnapshotDetail } from "./api";

export function SnapshotDetailPage() {
  const { snapshotId = "0" } = useParams();

  const detailQuery = useQuery({
    queryKey: ["snapshot", snapshotId],
    queryFn: () => getSnapshotDetail(snapshotId),
  });

  if (detailQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Snapshot Detail</h1>
        <p>Loading snapshot.</p>
      </section>
    );
  }

  if (detailQuery.isError || !detailQuery.data) {
    return (
      <section className="page-stack">
        <h1>Snapshot Detail</h1>
        <p role="alert">{detailQuery.error?.message || "Failed to load snapshot."}</p>
      </section>
    );
  }

  const snapshot = detailQuery.data;

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Snapshot {snapshot.id}</p>
          <h1>{snapshot.version}</h1>
          <p className="page-summary">
            Dataset{" "}
            <Link to={`/data/datasets/${snapshot.dataset_id}`} className="inline-link">
              {snapshot.dataset_name}
            </Link>
          </p>
        </div>
        <div className="hero-meter">
          <span>Annotations</span>
          <strong>{snapshot.annotation_count}</strong>
          <small>Project {snapshot.project_id}</small>
        </div>
      </header>

      <section className="panel">
        <div className="panel-header">
          <h2>Metadata</h2>
          {snapshot.based_on_snapshot_id ? (
            <Link to={`/data/diff?before=${snapshot.based_on_snapshot_id}&after=${snapshot.id}`}>
              Compare with previous
            </Link>
          ) : (
            <span />
          )}
        </div>
        <div className="data-meta">
          <span>Snapshot ID: {snapshot.id}</span>
          <span>
            Parent:{" "}
            {snapshot.based_on_snapshot_id ? `#${snapshot.based_on_snapshot_id}` : "No parent snapshot"}
          </span>
        </div>
        <p>{snapshot.note || "No note for this snapshot."}</p>
      </section>

      <section className="panel panel-accent">
        <p>
          Publish status is not wired in this slice yet. Use this page for metadata and diff inspection only.
        </p>
      </section>
    </section>
  );
}
