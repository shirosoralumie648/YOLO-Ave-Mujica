import { createBrowserRouter, Navigate } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";
import { TaskListPage } from "../features/tasks/task-list-page";
import { TaskDetailPage } from "../features/tasks/task-detail-page";

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
      { path: "tasks", element: <TaskListPage /> },
      { path: "tasks/:taskId", element: <TaskDetailPage /> },
    ],
  },
]);
