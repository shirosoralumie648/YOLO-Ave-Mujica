import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";

import { AppShell } from "./app-shell";

describe("AppShell", () => {
  it("renders the task-first navigation and keeps version context visible", () => {
    render(
      <MemoryRouter
        future={{ v7_relativeSplatPath: true, v7_startTransition: true }}
        initialEntries={["/projects/1/overview?snapshot_id=snap-009"]}
      >
        <Routes>
          <Route path="/projects/:projectId" element={<AppShell />}>
            <Route path="overview" element={<OutletContent />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("Project 1")).toBeInTheDocument();
    expect(screen.getAllByText("snap-009")).toHaveLength(2);
    expect(screen.getByRole("link", { name: /Overview/ })).toHaveAttribute(
      "href",
      "/projects/1/overview?snapshot_id=snap-009",
    );
    expect(screen.getByRole("link", { name: /Tasks/ })).toHaveAttribute(
      "href",
      "/projects/1/tasks?snapshot_id=snap-009",
    );
    expect(screen.getByText("Overview body")).toBeInTheDocument();
  });
});

function OutletContent() {
  return <div>Overview body</div>;
}
