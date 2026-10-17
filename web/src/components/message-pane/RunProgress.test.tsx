// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RunProgress, goalStatusLabel, goalUsageParts } from "./RunProgress";

afterEach(cleanup);

describe("RunProgress", () => {
  it("renders nothing without a goal or steps", () => {
    const { container } = render(<RunProgress plan={{ steps: [] }} />);
    expect(container.firstChild).toBeNull();
  });

  it("shows the live goal and expanded steps", () => {
    render(
      <RunProgress
        live
        goal={{ objective: "Make the tests pass", status: "active", tokens_used: 12_500, token_budget: 50_000, time_used_seconds: 125 }}
        plan={{
          explanation: "Fix then verify",
          steps: [
            { step: "Inspect failures", status: "completed" },
            { step: "Patch the parser", status: "in_progress" },
            { step: "Run the suite", status: "pending" }
          ]
        }}
      />
    );

    expect(screen.getByText("Pursuing goal")).toBeTruthy();
    expect(screen.getByText("Make the tests pass")).toBeTruthy();
    expect(screen.getByText("13k / 50k tokens · 2m 5s")).toBeTruthy();
    expect(screen.getByText("1/3")).toBeTruthy();
    expect(screen.getByText("Fix then verify")).toBeTruthy();
    expect(screen.getByText("Patch the parser").closest("li")?.getAttribute("data-status")).toBe("in_progress");
  });

  it("collapses finished steps to the current one until opened", () => {
    render(
      <RunProgress
        plan={{
          steps: [
            { step: "Inspect", status: "completed" },
            { step: "Patch", status: "in_progress" }
          ]
        }}
      />
    );

    expect(screen.queryByText("Inspect")).toBeNull();
    expect(screen.getByText("· Patch")).toBeTruthy();
    fireEvent.click(screen.getByText("Steps"));
    expect(screen.getByText("Inspect")).toBeTruthy();
  });

  it("labels goal outcomes", () => {
    expect(goalStatusLabel({ objective: "x", status: "complete" }).label).toBe("Goal achieved");
    expect(goalStatusLabel({ objective: "x", status: "budget_limited", token_budget: 10 }).label).toBe("Goal unmet");
    expect(goalStatusLabel({ objective: "x", status: "budget_limited" }).label).toBe("Goal abandoned");
    expect(goalStatusLabel({ objective: "x", status: "cleared" }).label).toBe("Goal cleared");
    expect(goalUsageParts({ objective: "x", status: "active", iterations: 2, time_used_seconds: 30 })).toEqual([
      "30s",
      "2 unmet checks"
    ]);
  });
});
