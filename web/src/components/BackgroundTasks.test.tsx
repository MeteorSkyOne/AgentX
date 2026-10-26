// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchBackgroundTaskOutput, listBackgroundTasks, stopBackgroundTask } from "../api/client";
import type { BackgroundTask } from "../api/types";
import {
  BackgroundTasks,
  formatTaskDuration,
  mergeAgentBackgroundTasks,
  visibleBackgroundTasks
} from "./BackgroundTasks";

vi.mock("../api/client", () => ({
  fetchBackgroundTaskOutput: vi.fn(),
  listBackgroundTasks: vi.fn(),
  stopBackgroundTask: vi.fn()
}));

afterEach(() => {
  cleanup();
  vi.mocked(fetchBackgroundTaskOutput).mockReset();
  vi.mocked(listBackgroundTasks).mockReset();
  vi.mocked(stopBackgroundTask).mockReset();
});

function task(overrides: Partial<BackgroundTask>): BackgroundTask {
  return {
    id: "b1",
    agent_id: "agt_1",
    kind: "shell",
    description: "npm test",
    status: "running",
    has_output: true,
    started_at: new Date(Date.now() - 65_000).toISOString(),
    ...overrides
  };
}

describe("background task helpers", () => {
  it("replaces only the updated agent's tasks, oldest first", () => {
    const current = [
      task({ id: "a", agent_id: "agt_1", started_at: "2026-10-26T10:00:00Z" }),
      task({ id: "b", agent_id: "agt_2", started_at: "2026-10-26T10:01:00Z" })
    ];
    const merged = mergeAgentBackgroundTasks(current, "agt_1", [
      task({ id: "c", agent_id: "agt_1", started_at: "2026-10-26T10:02:00Z" })
    ]);
    expect(merged.map((item) => item.id)).toEqual(["b", "c"]);
    expect(mergeAgentBackgroundTasks(undefined, "agt_1", [])).toEqual([]);
  });

  it("hides tasks that ended a while ago", () => {
    const now = Date.parse("2026-10-26T12:00:00Z");
    const tasks = [
      task({ id: "running" }),
      task({ id: "recent", status: "completed", ended_at: "2026-10-26T11:50:00Z" }),
      task({ id: "old", status: "completed", ended_at: "2026-10-26T10:00:00Z" })
    ];
    expect(visibleBackgroundTasks(tasks, now).map((item) => item.id)).toEqual(["running", "recent"]);
  });

  it("formats durations compactly", () => {
    expect(formatTaskDuration(9_400)).toBe("9s");
    expect(formatTaskDuration(65_000)).toBe("1m 05s");
    expect(formatTaskDuration(3_725_000)).toBe("1h 02m");
  });
});

function renderTasks() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <BackgroundTasks
        conversation={{ type: "channel", id: "chn_1" }}
        agents={[
          { id: "agt_1", name: "Claude" },
          { id: "agt_2", name: "Codex" }
        ]}
      />
    </QueryClientProvider>
  );
}

describe("BackgroundTasks", () => {
  it("renders nothing without tasks", async () => {
    vi.mocked(listBackgroundTasks).mockResolvedValue([]);
    renderTasks();
    await waitFor(() => expect(listBackgroundTasks).toHaveBeenCalledWith("channel", "chn_1"));
    expect(screen.queryByText("Background tasks")).toBeNull();
  });

  it("summarizes, expands, shows output and stops a running task", async () => {
    vi.mocked(listBackgroundTasks).mockResolvedValue([
      task({ id: "m1", kind: "monitor", description: "Watch deploy log" }),
      task({
        id: "a1",
        agent_id: "agt_2",
        kind: "agent",
        description: "Research",
        status: "completed",
        has_output: false,
        summary: "Found three call sites",
        ended_at: new Date().toISOString()
      })
    ]);
    vi.mocked(fetchBackgroundTaskOutput).mockResolvedValue({ output: "deploy 42% done\n", truncated: true });
    vi.mocked(stopBackgroundTask).mockResolvedValue(undefined);
    renderTasks();

    const toggle = await screen.findByRole("button", { name: /Background tasks/ });
    expect(toggle.textContent).toContain("1 running");
    expect(toggle.textContent).toContain("1 finished");
    expect(screen.queryByText("Watch deploy log")).toBeNull();

    fireEvent.click(toggle);
    expect(screen.getByText("Watch deploy log")).toBeTruthy();
    expect(screen.getByText("Found three call sites")).toBeTruthy();
    expect(screen.getByText("Codex")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Stop task" })).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Show output" }));
    expect(await screen.findByText("deploy 42% done")).toBeTruthy();
    expect(screen.getByText("Earlier output omitted.")).toBeTruthy();
    expect(fetchBackgroundTaskOutput).toHaveBeenCalledWith("channel", "chn_1", "agt_1", "m1");

    fireEvent.click(screen.getByRole("button", { name: "Stop task" }));
    await waitFor(() => expect(stopBackgroundTask).toHaveBeenCalledWith("channel", "chn_1", "agt_1", "m1"));
  });
});
