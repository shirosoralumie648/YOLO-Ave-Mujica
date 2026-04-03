import { createBrowserRouter } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";
import { TaskListPage } from "../features/tasks/task-list-page";
import { TaskDetailPage } from "../features/tasks/task-detail-page";
import { DatasetListPage } from "../features/data/dataset-list-page";
import { DatasetDetailPage } from "../features/data/dataset-detail-page";
import { SnapshotDetailPage } from "../features/data/snapshot-detail-page";
import { SnapshotDiffPage } from "../features/data/snapshot-diff-page";
import { ReviewQueuePage } from "../features/review/review-queue-page";
import { PublishCandidatesPage } from "../features/publish/publish-candidates-page";
import { PublishBatchDetailPage } from "../features/publish/publish-batch-detail-page";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <TaskOverviewPage /> },
      { path: "tasks", element: <TaskListPage /> },
      { path: "tasks/:taskId", element: <TaskDetailPage /> },
      { path: "data", element: <DatasetListPage /> },
      { path: "data/datasets/:datasetId", element: <DatasetDetailPage /> },
      { path: "data/snapshots/:snapshotId", element: <SnapshotDetailPage /> },
      { path: "data/diff", element: <SnapshotDiffPage /> },
      { path: "review", element: <ReviewQueuePage /> },
      { path: "publish/candidates", element: <PublishCandidatesPage /> },
      { path: "publish/batches/:batchId", element: <PublishBatchDetailPage /> },
    ],
  },
]);
