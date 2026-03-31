import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppShell } from "../../app/layout/app-shell";

describe("AppShell", () => {
  it("renders primary navigation links", () => {
    render(
      <MemoryRouter initialEntries={["/projects/1/overview"]}>
        <Routes>
          <Route path="/projects/:projectId" element={<AppShell />}>
            <Route path="overview" element={<div>Overview Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: /overview/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /tasks/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /data/i })).toBeInTheDocument();
  });
});
