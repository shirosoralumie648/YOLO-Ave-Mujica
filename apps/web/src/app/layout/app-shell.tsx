import { NavLink, Outlet, useLocation, useParams, useSearchParams } from "react-router-dom";

const NAV_ITEMS = [
  { label: "Overview", section: "overview" },
  { label: "Data", section: "data" },
  { label: "Tasks", section: "tasks" },
  { label: "Review", section: "review" },
  { label: "Training", section: "training" },
  { label: "Artifacts", section: "artifacts" },
];

export function AppShell() {
  const { projectId = "1" } = useParams();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const snapshotID = searchParams.get("snapshot_id");
  const currentSection = NAV_ITEMS.find((item) => location.pathname.includes(`/${item.section}`))?.label ?? "Overview";
  const preservedSearch = searchParams.toString();

  return (
    <div className="app-shell">
      <aside className="sidebar panel">
        <div className="brand">
          <p className="eyebrow">YOLO Toolchain</p>
          <h1>Task-first Console</h1>
          <p className="brand-copy">
            把数据、标注、审核和训练链路放到同一个任务入口里。
          </p>
        </div>

        <nav className="nav-grid" aria-label="Primary">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.section}
              className={({ isActive }) => (isActive ? "nav-link active" : "nav-link")}
              to={buildProjectPath(projectId, item.section, preservedSearch)}
            >
              <span>{item.label}</span>
              <small>{navHint(item.section)}</small>
            </NavLink>
          ))}
        </nav>

        <div className="sidebar-footer">
          <div className="metric-chip">
            <span className="metric-label">Project</span>
            <strong>{projectId}</strong>
          </div>
          {snapshotID ? (
            <div className="metric-chip accent">
              <span className="metric-label">Snapshot</span>
              <strong>{snapshotID}</strong>
            </div>
          ) : null}
        </div>
      </aside>

      <main className="content">
        <header className="topbar panel">
          <div>
            <p className="eyebrow">Default Entry</p>
            <h2>{currentSection}</h2>
          </div>
          <div className="context-strip" aria-label="Context">
            <span className="context-pill">Project {projectId}</span>
            {snapshotID ? <span className="context-pill muted">{snapshotID}</span> : null}
            <span className="context-pill muted">Task-first routing</span>
          </div>
        </header>

        <div className="content-body">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

function buildProjectPath(projectID: string, section: string, search: string): string {
  const base = `/projects/${projectID}/${section}`;
  return search ? `${base}?${search}` : base;
}

function navHint(section: string): string {
  switch (section) {
  case "overview":
    return "blockers + idle work";
  case "data":
    return "datasets + snapshots";
  case "tasks":
    return "queue + assignments";
  case "review":
    return "accept / reject";
  case "training":
    return "runs + compare";
  case "artifacts":
    return "promotion + delivery";
  default:
    return "";
  }
}
