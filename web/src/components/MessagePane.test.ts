// @vitest-environment jsdom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { ConversationAgentContext, Message } from "@/api/types";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";

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
  onOpenWorkspacePath,
}: {
  messages: Message[];
  streaming: Array<{
    runID: string;
    agentID?: string;
    text: string;
    error?: string;
  }>;
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
        preferences: { show_ttft: false, show_tps: false },
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
