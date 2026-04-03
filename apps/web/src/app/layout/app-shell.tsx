import { NavLink, Outlet, useLocation } from "react-router-dom";

export function AppShell() {
  const location = useLocation();
  const isWorkspaceRoute = location.pathname.includes("/workspace");

  return (
    <div className="app-shell">
      <aside className="shell-sidebar">
        <div className="shell-brand">
          <span className="shell-eyebrow">YOLO Platform</span>
          <strong>Operations Shell</strong>
        </div>
        <nav className="shell-nav" aria-label="Primary">
          <NavLink to="/">Overview</NavLink>
          <NavLink to="/tasks">Tasks</NavLink>
          <NavLink to="/review">Review</NavLink>
          <NavLink to="/publish/candidates">Publish</NavLink>
          <NavLink to="/data">Data</NavLink>
        </nav>
      </aside>
      <main className={isWorkspaceRoute ? "shell-content shell-content--workspace" : "shell-content"}>
        <header className={isWorkspaceRoute ? "shell-context shell-context--workspace" : "shell-context"}>
          <span>{isWorkspaceRoute ? "Workspace Session" : "Project Context"}</span>
          <strong>{isWorkspaceRoute ? "Annotation Flow" : "Project 1"}</strong>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
