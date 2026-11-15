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
  it("replaces the updated agent's tasks and drops non-running ones, oldest first", () => {
    const current = [
      task({ id: "a", agent_id: "agt_1", started_at: "2026-10-26T10:00:00Z" }),
      task({ id: "b", agent_id: "agt_2", started_at: "2026-10-26T10:01:00Z" })
    ];
    const merged = mergeAgentBackgroundTasks(current, "agt_1", [
      task({ id: "c", agent_id: "agt_1", started_at: "2026-10-26T10:02:00Z" }),
      task({ id: "done", agent_id: "agt_1", status: "completed", started_at: "2026-10-26T10:03:00Z" })
    ]);
    expect(merged.map((item) => item.id)).toEqual(["b", "c"]);
    expect(mergeAgentBackgroundTasks(merged, "agt_1", [{ ...merged[1], status: "completed" }]).map((item) => item.id)).toEqual(["b"]);
    expect(mergeAgentBackgroundTasks(undefined, "agt_1", [])).toEqual([]);
  });

  it("hides completed and stopped tasks immediately", () => {
    const tasks = [
      task({ id: "running" }),
      task({ id: "completed", status: "completed", ended_at: "2026-10-26T11:59:59Z" }),
      task({ id: "stopped", status: "stopped", ended_at: "2026-10-26T11:59:59Z" }),
      task({ id: "failed", status: "failed", ended_at: "2026-10-26T11:59:59Z" })
    ];
    expect(visibleBackgroundTasks(tasks).map((item) => item.id)).toEqual(["running"]);
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
  return queryClient;
}

describe("BackgroundTasks", () => {
  it("renders nothing when no tasks are running", async () => {
    vi.mocked(listBackgroundTasks).mockResolvedValue([
      task({ id: "done", status: "completed", ended_at: new Date().toISOString() })
    ]);
    renderTasks();
    await waitFor(() => expect(listBackgroundTasks).toHaveBeenCalledWith("channel", "chn_1"));
    expect(screen.queryByText("Background tasks")).toBeNull();
  });

  it("shows running tasks expanded by default, displays output, and clears on manual stop", async () => {
    vi.mocked(listBackgroundTasks).mockResolvedValue([
      task({ id: "m1", kind: "monitor", description: "Watch deploy log" }),
      task({
        id: "a1",
        agent_id: "agt_2",
        kind: "agent",
        description: "Completed research",
        status: "completed",
        has_output: false,
        ended_at: new Date().toISOString()
      })
    ]);
    vi.mocked(fetchBackgroundTaskOutput).mockResolvedValue({ output: "deploy 42% done\n", truncated: true });
    vi.mocked(stopBackgroundTask).mockResolvedValue(undefined);
    renderTasks();

    const toggle = await screen.findByRole("button", { name: /Background tasks/ });
    expect(toggle.textContent).toContain("1 running");
    expect(toggle.textContent).not.toContain("finished");
    // Expanded by default so the running task and manual Stop button are immediately accessible.
    expect(screen.getByText("Watch deploy log")).toBeTruthy();
    expect(screen.queryByText("Completed research")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Show output" }));
    expect(await screen.findByText("deploy 42% done")).toBeTruthy();
    expect(screen.getByText("Earlier output omitted.")).toBeTruthy();
    expect(fetchBackgroundTaskOutput).toHaveBeenCalledWith("channel", "chn_1", "agt_1", "m1");

    // Can collapse and re-expand.
    fireEvent.click(toggle);
    expect(screen.queryByText("Watch deploy log")).toBeNull();
    fireEvent.click(toggle);
    expect(screen.getByText("Watch deploy log")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Stop task" }));
    await waitFor(() => expect(stopBackgroundTask).toHaveBeenCalledWith("channel", "chn_1", "agt_1", "m1"));
    await waitFor(() => expect(screen.queryByText("Background tasks")).toBeNull());
  });
});
