// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type {
  Agent,
  Channel,
  Project,
  ScheduledTask,
  ScheduledTaskRun,
  Thread,
  Workspace,
} from "../../api/types";
import {
  channelThreads,
  createScheduledTask,
  scheduledTaskRuns,
  scheduledTasks,
} from "../../api/client";
import { TasksPanel } from "./TasksPanel";

vi.mock("../../api/client", () => ({
  channelThreads: vi.fn(async () => []),
  createScheduledTask: vi.fn(async () => forumTask()),
  deleteScheduledTask: vi.fn(async () => undefined),
  runScheduledTask: vi.fn(async () => forumRun()),
  scheduledTaskRuns: vi.fn(async () => []),
  scheduledTasks: vi.fn(async () => []),
  updateScheduledTask: vi.fn(async () => forumTask()),
}));

afterEach(cleanup);

beforeEach(() => {
  vi.mocked(scheduledTasks).mockResolvedValue([]);
  vi.mocked(scheduledTaskRuns).mockResolvedValue([]);
  vi.mocked(channelThreads).mockResolvedValue([]);
  vi.mocked(createScheduledTask).mockClear();
});

describe("TasksPanel", () => {
  it("creates a forum post task targeting the selected forum channel", async () => {
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /new/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Daily report" } });
    fireEvent.change(screen.getByLabelText("Type"), { target: { value: "forum_post" } });

    const forumSelect = await screen.findByLabelText("Forum");
    fireEvent.change(forumSelect, { target: { value: "chn_forum" } });
    fireEvent.change(screen.getByLabelText("Post title"), { target: { value: "Report ${date}" } });
    fireEvent.change(screen.getByLabelText("Prompt"), { target: { value: "write the report" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(createScheduledTask).toHaveBeenCalledTimes(1));
    expect(createScheduledTask).toHaveBeenCalledWith(
      "prj_1",
      expect.objectContaining({
        kind: "forum_post",
        conversation_type: "channel",
        conversation_id: "chn_forum",
        post_title: "Report ${date}",
        prompt: "write the report",
        fresh_context: false,
        notify: true,
      })
    );
  });

  it("sends notify=false when the reply notification is turned off", async () => {
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /new/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Quiet upkeep" } });
    fireEvent.click(screen.getByText("Notify on reply"));
    fireEvent.change(screen.getByLabelText("Prompt"), { target: { value: "hi" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(createScheduledTask).toHaveBeenCalledTimes(1));
    expect(createScheduledTask).toHaveBeenCalledWith(
      "prj_1",
      expect.objectContaining({ kind: "agent_prompt", notify: false })
    );
  });

  it("sends fresh_context for agent prompt tasks", async () => {
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /new/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Fresh ping" } });
    fireEvent.click(screen.getByText("Fresh context"));
    fireEvent.change(screen.getByLabelText("Prompt"), { target: { value: "ping" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(createScheduledTask).toHaveBeenCalledTimes(1));
    expect(createScheduledTask).toHaveBeenCalledWith(
      "prj_1",
      expect.objectContaining({
        kind: "agent_prompt",
        conversation_type: "channel",
        conversation_id: "chn_text",
        fresh_context: true,
      })
    );
  });

  it("opens the forum post created by a run", async () => {
    const task = forumTask();
    const run = forumRun();
    const thread = forumThread();
    vi.mocked(scheduledTasks).mockResolvedValue([task]);
    vi.mocked(scheduledTaskRuns).mockResolvedValue([run]);
    vi.mocked(channelThreads).mockResolvedValue([thread]);
    const onOpenThread = vi.fn();

    renderPanel({ onOpenThread });

    const openButton = await screen.findByRole("button", { name: "Open post" });
    fireEvent.click(openButton);

    await waitFor(() => expect(onOpenThread).toHaveBeenCalledWith(thread));
    expect(channelThreads).toHaveBeenCalledWith("chn_forum");
  });

  it("shows the full run history in a dialog", async () => {
    const task = forumTask();
    vi.mocked(scheduledTasks).mockResolvedValue([task]);
    vi.mocked(scheduledTaskRuns).mockResolvedValue([
      { ...forumRun(), status: "failed", error: "boom" },
    ]);

    renderPanel();

    fireEvent.click(await screen.findByLabelText("Run history"));

    await waitFor(() => expect(screen.getByText(/last 100 runs/)).toBeTruthy());
    expect(scheduledTaskRuns).toHaveBeenCalledWith("tsk_1", 100);
    expect(screen.getAllByText("boom").length).toBeGreaterThan(0);
  });
});

function renderPanel(overrides: { onOpenThread?: (thread: Thread) => void } = {}) {
  return render(
    <Providers>
      <TasksPanel
        project={project()}
        projectWorkspace={workspace()}
        channels={[textChannel(), forumChannel()]}
        threads={[]}
        agents={[agent()]}
        onOpenChannel={() => undefined}
        onOpenThread={overrides.onOpenThread ?? (() => undefined)}
      />
    </Providers>
  );
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function project(): Project {
  return {
    id: "prj_1",
    organization_id: "org_1",
    name: "Project",
    workspace_id: "wsp_1",
    created_by: "usr_1",
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function workspace(): Workspace {
  return {
    id: "wsp_1",
    organization_id: "org_1",
    type: "local",
    name: "Workspace",
    path: "/tmp/workspace",
    created_by: "usr_1",
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function textChannel(): Channel {
  return {
    id: "chn_text",
    organization_id: "org_1",
    project_id: "prj_1",
    type: "text",
    name: "general",
    team_max_batches: 3,
    team_max_runs: 12,
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function forumChannel(): Channel {
  return { ...textChannel(), id: "chn_forum", type: "thread", name: "reports" };
}

function forumThread(): Thread {
  return {
    id: "thr_1",
    organization_id: "org_1",
    project_id: "prj_1",
    channel_id: "chn_forum",
    title: "Report 2026-07-30",
    created_by: "usr_1",
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function agent(): Agent {
  return {
    id: "agt_1",
    organization_id: "org_1",
    bot_user_id: "bot_1",
    kind: "fake",
    name: "Agent",
    handle: "agent",
    description: "",
    model: "",
    effort: "",
    config_workspace_id: "wsp_1",
    default_workspace_id: "wsp_1",
    enabled: true,
    fast_mode: false,
    yolo_mode: false,
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function forumTask(): ScheduledTask {
  return {
    id: "tsk_1",
    organization_id: "org_1",
    project_id: "prj_1",
    name: "Daily report",
    kind: "forum_post",
    enabled: true,
    schedule: "0 9 * * *",
    timezone: "UTC",
    conversation_type: "channel",
    conversation_id: "chn_forum",
    prompt: "write the report",
    post_title: "Report ${date}",
    fresh_context: false,
    notify: true,
    timeout_seconds: 600,
    created_by: "usr_1",
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function forumRun(): ScheduledTaskRun {
  return {
    id: "trn_1",
    task_id: "tsk_1",
    organization_id: "org_1",
    project_id: "prj_1",
    kind: "forum_post",
    trigger: "scheduled",
    started_at: "2026-07-30T09:00:00Z",
    finished_at: "2026-07-30T09:00:20Z",
    status: "completed",
    output_truncated: false,
    message_id: "msg_1",
    thread_id: "thr_1",
  };
}
