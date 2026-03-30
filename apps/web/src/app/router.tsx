import { createBrowserRouter, Navigate, Outlet } from "react-router-dom";

import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";
import { TaskListPage } from "../features/tasks/task-list-page";
import { TaskDetailPage } from "../features/tasks/task-detail-page";

export const appRouter = createBrowserRouter([
  {
    path: "/",
    element: <Navigate to="/projects/1/overview" replace />,
  },
  {
    path: "/projects/:projectId",
    element: <AppShell />,
    children: [
      {
        path: "overview",
        element: <TaskOverviewPage />,
      },
      {
        path: "data",
        element: <PlaceholderPage title="Data" description="Data Hub will land here next." />,
      },
      {
        path: "tasks",
        element: <TaskListPage />,
      },
      {
        path: "tasks/:taskId",
        element: <TaskDetailPage />,
      },
      {
        path: "review",
        element: <PlaceholderPage title="Review" description="Review queue will appear here." />,
      },
      {
        path: "training",
        element: <PlaceholderPage title="Training" description="Training runs and evaluation compare will appear here." />,
      },
      {
        path: "artifacts",
        element: <PlaceholderPage title="Artifacts" description="Recommended model artifacts will appear here." />,
      },
      {
        index: true,
        element: <Navigate to="overview" replace />,
      },
    ],
  },
]);

function PlaceholderPage(props: { title: string; description: string }) {
  return (
    <section className="panel page-intro">
      <p className="eyebrow">Phase 1</p>
      <h1>{props.title}</h1>
      <p>{props.description}</p>
      <Outlet />
    </section>
  );
}
