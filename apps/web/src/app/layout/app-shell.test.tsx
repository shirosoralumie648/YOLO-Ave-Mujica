import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";

import { AppShell } from "./app-shell";

describe("AppShell", () => {
  it("renders root-scoped navigation and project context", () => {
    render(
      <MemoryRouter initialEntries={["/tasks?status=blocked"]}>
        <Routes>
          <Route path="/" element={<AppShell />}>
            <Route path="tasks" element={<OutletContent />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("Project 1")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Overview/ })).toHaveAttribute("href", "/");
    expect(screen.getByRole("link", { name: /Tasks/ })).toHaveAttribute("href", "/tasks");
    expect(screen.getByText("Tasks body")).toBeInTheDocument();
  });
});

function OutletContent() {
  return <div>Tasks body</div>;
}
