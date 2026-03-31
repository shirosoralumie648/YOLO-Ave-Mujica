import { NavLink, Outlet } from "react-router-dom";

export function AppShell() {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">YOLO Platform</div>
        <nav className="nav">
          <NavLink to="/projects/1/overview">Overview</NavLink>
          <NavLink to="/projects/1/tasks">Tasks</NavLink>
          <NavLink to="/projects/1/data">Data</NavLink>
        </nav>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
