import { NavLink, Outlet } from "react-router-dom";

export function AppShell() {
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
      <main className="shell-content">
        <header className="shell-context">
          <span>Project Context</span>
          <strong>Project 1</strong>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
