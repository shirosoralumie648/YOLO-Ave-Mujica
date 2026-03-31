import { createBrowserRouter, Navigate } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";

function PlaceholderPage({ title }: { title: string }) {
  return (
    <section>
      <h1>{title}</h1>
      <p>Page scaffold.</p>
    </section>
  );
}

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Navigate to="/projects/1/overview" replace />,
  },
  {
    path: "/projects/:projectId",
    element: <AppShell />,
    children: [
      { path: "overview", element: <TaskOverviewPage /> },
      { path: "tasks", element: <PlaceholderPage title="Tasks" /> },
      { path: "tasks/:taskId", element: <PlaceholderPage title="Task Detail" /> },
    ],
  },
]);
