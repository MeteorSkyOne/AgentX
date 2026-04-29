// @vitest-environment jsdom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { ConversationAgentContext, Message, ProcessItem, UserPreferences } from "@/api/types";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import { fetchMessageProcessItem } from "../api/client";

vi.mock("../api/client", () => ({
  fetchAttachmentBlob: vi.fn(),
  fetchMessageProcessItem: vi.fn(),
}));

vi.mock("./MarkdownRenderer", async () => {
  const React = await import("react");
  return {
    MarkdownRenderer: ({
      text,
      workspacePath,
      onOpenWorkspacePath,
    }: {
      text: string;
      workspacePath?: string;
      onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
    }) => {
      const label = workspacePath
        ? text.split(/\s+/).find((part) => /[A-Za-z0-9._/-]+\.[A-Za-z0-9._-]+/.test(part))
        : undefined;
      const target = label ? { path: label.replace(/^\.\//, ""), label } : null;
      return React.createElement(
        "button",
        {
          type: "button",
          "data-testid": "mock-markdown",
          "data-workspace": workspacePath ?? "",
          "data-target": target?.path ?? "",
          onClick: () => {
            if (target) onOpenWorkspacePath?.(target);
          },
        },
        text
      );
    },
  };
});

import {
  MessagePane,
  createReadOnlyAttachmentEditorController,
  imageAttachmentPreviewDialogLabel,
  isTextAttachmentPreviewSupported,
  nextImagePreviewPan,
  nextImagePreviewScale,
} from "./MessagePane";

const mountedRoots: Array<{ root: Root; container: HTMLDivElement }> = [];

beforeAll(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  Element.prototype.scrollIntoView = vi.fn();
  class TestResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(window, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: TestResizeObserver,
  });
});

afterEach(() => {
  vi.clearAllMocks();
  for (const { root, container } of mountedRoots.splice(0)) {
    act(() => root.unmount());
    container.remove();
  }
});

describe("createReadOnlyAttachmentEditorController", () => {
  it("builds a readonly workspace editor controller for attachment previews", async () => {
    const controller = createReadOnlyAttachmentEditorController(
      { filename: "notes.ts" },
      "const value = 1;\n"
    );

    expect(controller.filePath).toBe("notes.ts");
    expect(controller.trimmedPath).toBe("notes.ts");
    expect(controller.fileBody).toBe("const value = 1;\n");
    expect(controller.canUseWorkspace).toBe(false);
    expect(controller.fileLoading).toBe(false);
    expect(controller.fileLoadError).toBeNull();

    controller.setFileBody("mutated");
    expect(controller.fileBody).toBe("const value = 1;\n");
    await expect(controller.saveFile()).resolves.toBeUndefined();
  });
});

describe("MessagePane workspace links", () => {
  it("passes workspace path open behavior to persisted messages", async () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = await renderMessagePane({
      messages: [
        message({
          id: "message-1",
          body: "See web/src/App.tsx",
          sender_type: "user",
          sender_id: "user-1",
        }),
      ],
      streaming: [],
      onOpenWorkspacePath: (target) => opened.push(target),
    });

    const button = markdownButtons(container)[0];
    expect(button.dataset.workspace).toBe("/workspace/AgentX");
    click(button);

    expect(opened).toEqual([{ path: "web/src/App.tsx", label: "web/src/App.tsx" }]);
  });

  it("passes workspace path open behavior to streaming messages", async () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = await renderMessagePane({
      messages: [],
      streaming: [
        {
          runID: "run-1",
          agentID: "agent-1",
          text: "Streaming web/src/App.tsx",
        },
      ],
      onOpenWorkspacePath: (target) => opened.push(target),
    });

    const button = markdownButtons(container)[0];
    expect(button.dataset.workspace).toBe("/workspace/AgentX");
    click(button);

    expect(opened).toEqual([{ path: "web/src/App.tsx", label: "web/src/App.tsx" }]);
  });
});

describe("MessagePane process details", () => {
  it("loads persisted tool details only when a tool row is opened", async () => {
    const fetchProcess = vi.mocked(fetchMessageProcessItem);
    fetchProcess.mockResolvedValueOnce({
      item: {
        type: "tool_call",
        tool_name: "Bash",
        tool_call_id: "call_1",
        input: { command: "pnpm test" },
        raw: { type: "tool_use" },
      },
      result: {
        type: "tool_result",
        tool_call_id: "call_1",
        output: "tests passed",
        raw: { type: "tool_result" },
      },
    });

    const { container } = await renderMessagePane({
      messages: [
        message({
          id: "msg-bot",
          sender_type: "bot",
          sender_id: "bot-1",
          metadata: {
            process: [
              {
                type: "tool_call",
                tool_name: "Bash",
                tool_call_id: "call_1",
                process_index: 0,
                has_detail: true,
              },
              {
                type: "tool_result",
                tool_call_id: "call_1",
                status: "completed",
                process_index: 1,
                has_detail: true,
              },
            ],
          },
        }),
      ],
      streaming: [],
      onOpenWorkspacePath: () => undefined,
    });

    expect(container.textContent).not.toContain("pnpm test");
    expect(fetchProcess).not.toHaveBeenCalled();

    click(buttonWithText(container, "Process"));
    await clickAsync(buttonWithText(container, "Tool"));

    expect(fetchProcess).toHaveBeenCalledTimes(1);
    expect(fetchProcess).toHaveBeenCalledWith("msg-bot", 0);
    expect(container.textContent).toContain("pnpm test");
    expect(container.textContent).toContain("tests passed");

    click(buttonWithText(container, "Tool"));
    await clickAsync(buttonWithText(container, "Tool"));
    expect(fetchProcess).toHaveBeenCalledTimes(1);
  });

  it("uses streaming tool details without lazy requests", async () => {
    const fetchProcess = vi.mocked(fetchMessageProcessItem);
    const { container } = await renderMessagePane({
      messages: [],
      streaming: [
        {
          runID: "run-1",
          agentID: "agent-1",
          text: "working",
          process: [
            {
              type: "tool_call",
              tool_name: "Bash",
              tool_call_id: "call_1",
              input: { command: "go test ./..." },
            },
            {
              type: "tool_result",
              tool_call_id: "call_1",
              output: "ok",
            },
          ],
        },
      ],
      onOpenWorkspacePath: () => undefined,
    });

    click(buttonWithText(container, "Tool"));

    expect(fetchProcess).not.toHaveBeenCalled();
    expect(container.textContent).toContain("go test ./...");
    expect(container.textContent).toContain("ok");
  });
});

describe("MessagePane avatar density", () => {
  it("hides persisted and streaming message avatars when enabled", async () => {
    const { container } = await renderMessagePane({
      messages: [
        message({ id: "user-message", sender_type: "user", body: "user body" }),
        message({ id: "bot-message", sender_type: "bot", sender_id: "bot-1", body: "bot body" }),
      ],
      streaming: [{ runID: "run-1", agentID: "agent-1", text: "streaming body" }],
      preferences: { show_ttft: false, show_tps: false, hide_avatars: true },
      onOpenWorkspacePath: () => undefined,
    });

    expect(container.textContent).toContain("user body");
    expect(container.textContent).toContain("bot body");
    expect(container.textContent).toContain("streaming body");
    expect(container.querySelector('[data-slot="avatar"]')).toBeNull();
  });
});

describe("isTextAttachmentPreviewSupported", () => {
  it("previews text attachments in the readonly editor", () => {
    expect(
      isTextAttachmentPreviewSupported({ kind: "text", content_type: "text/plain" })
    ).toBe(true);
  });

  it("does not send image attachments through the text editor", () => {
    expect(
      isTextAttachmentPreviewSupported({ kind: "image", content_type: "image/png" })
    ).toBe(false);
  });
});

describe("imageAttachmentPreviewDialogLabel", () => {
  it("uses filename and metadata for image preview modal labels", () => {
    expect(
      imageAttachmentPreviewDialogLabel({
        filename: "diagram.png",
        content_type: "image/png",
        size_bytes: 2048,
      })
    ).toEqual({
      title: "diagram.png",
      description: "image/png · 2 KB",
    });
  });
});

describe("nextImagePreviewScale", () => {
  it("zooms in and out from wheel delta", () => {
    expect(nextImagePreviewScale(1, -600)).toBeGreaterThan(1);
    expect(nextImagePreviewScale(1, 600)).toBeLessThan(1);
  });

  it("clamps zoom bounds", () => {
    expect(nextImagePreviewScale(6, -600)).toBe(6);
    expect(nextImagePreviewScale(0.25, 600)).toBe(0.25);
  });
});

describe("nextImagePreviewPan", () => {
  it("moves a zoomed image by the pointer delta", () => {
    expect(nextImagePreviewPan({ x: 10, y: -5 }, 24.25, -12.5, 2)).toEqual({
      x: 34.25,
      y: -17.5,
    });
  });

  it("resets pan when the image is not zoomed in", () => {
    expect(nextImagePreviewPan({ x: 10, y: 20 }, 5, 5, 1)).toEqual({ x: 0, y: 0 });
  });
});

async function renderMessagePane({
  messages,
  streaming,
  preferences = { show_ttft: false, show_tps: false, hide_avatars: false },
  onOpenWorkspacePath,
}: {
  messages: Message[];
  streaming: Array<{
    runID: string;
    agentID?: string;
    text: string;
    error?: string;
    process?: ProcessItem[];
  }>;
  preferences?: UserPreferences;
  onOpenWorkspacePath: (target: WorkspacePathTarget) => void;
}): Promise<{ container: HTMLDivElement }> {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ root, container });

  await act(async () => {
    root.render(
      createElement(MessagePane, {
        messages,
        isLoading: false,
        isLoadingOlder: false,
        hasOlderMessages: false,
        streaming,
        agents: [agentContext()],
        preferences,
        theme: "light",
        onUpdateMessage: async (_messageID: string, body: string) => message({ body }),
        onDeleteMessage: async () => undefined,
        onReplyMessage: () => undefined,
        onLoadOlder: () => false,
        workspacePath: "/workspace/AgentX",
        onOpenWorkspacePath,
      })
    );
    await vi.dynamicImportSettled();
    await Promise.resolve();
  });

  return { container };
}

function markdownButtons(container: HTMLElement): HTMLButtonElement[] {
  const buttons = Array.from(
    container.querySelectorAll<HTMLButtonElement>('button[data-testid="mock-markdown"]')
  );
  expect(buttons.length).toBeGreaterThan(0);
  return buttons;
}

function click(element: HTMLElement) {
  act(() => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
  });
}

async function clickAsync(element: HTMLElement) {
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    await Promise.resolve();
  });
}

function buttonWithText(container: HTMLElement, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find((item) =>
    item.textContent?.includes(text)
  );
  expect(button).toBeTruthy();
  return button!;
}

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "message",
    organization_id: "org-1",
    conversation_type: "channel",
    conversation_id: "channel-1",
    sender_type: "user",
    sender_id: "user-1",
    kind: "text",
    body: "body",
    created_at: "2026-04-28T00:00:00Z",
    ...overrides,
  };
}

function agentContext(): ConversationAgentContext {
  const workspace = {
    id: "workspace-1",
    organization_id: "org-1",
    type: "project",
    name: "Workspace",
    path: "/workspace/AgentX",
    created_by: "user-1",
    created_at: "2026-04-28T00:00:00Z",
    updated_at: "2026-04-28T00:00:00Z",
  };
  return {
    binding: {
      channel_id: "channel-1",
      agent_id: "agent-1",
      created_at: "2026-04-28T00:00:00Z",
      updated_at: "2026-04-28T00:00:00Z",
    },
    agent: {
      id: "agent-1",
      organization_id: "org-1",
      bot_user_id: "bot-1",
      kind: "fake",
      name: "Agent",
      handle: "agent",
      description: "",
      model: "",
      effort: "",
      config_workspace_id: "workspace-1",
      default_workspace_id: "workspace-1",
      enabled: true,
      fast_mode: false,
      yolo_mode: false,
      created_at: "2026-04-28T00:00:00Z",
      updated_at: "2026-04-28T00:00:00Z",
    },
    config_workspace: workspace,
    run_workspace: workspace,
  };
}
