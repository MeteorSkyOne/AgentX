// @vitest-environment jsdom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { ProcessItem } from "@/api/types";

vi.mock("./markdown", async () => {
  const React = await import("react");
  return {
    messageBodyClassName: "message-body",
    MessageMarkdown: ({ text }: { text: string }) =>
      React.createElement("div", { "data-testid": "mock-markdown" }, text),
  };
});

import { InterleavedMessageBody } from "./ProcessTimeline";

let container: HTMLDivElement | null = null;
let root: Root | null = null;

beforeAll(() => {
  class TestResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver = TestResizeObserver as unknown as typeof ResizeObserver;
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
});

afterEach(() => {
  act(() => {
    root?.unmount();
  });
  container?.remove();
  root = null;
  container = null;
});

function render(process: ProcessItem[], text = "done") {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => {
    root!.render(
      createElement(InterleavedMessageBody, {
        text,
        processItems: process,
        defaultProcessOpen: true,
      })
    );
  });
  return container!;
}

describe("subagent rendering", () => {
  it("shows a subagent's work as its own group instead of flattening it", () => {
    const el = render([
      {
        type: "tool_call",
        tool_name: "Agent",
        tool_call_id: "toolu_parent",
        input: { description: "Count Go files" },
      },
      {
        type: "tool_call",
        tool_name: "Bash",
        tool_call_id: "toolu_child",
        parent_tool_call_id: "toolu_parent",
        input: { command: "find . -name '*.go'" },
      },
      { type: "tool_result", tool_call_id: "toolu_child", parent_tool_call_id: "toolu_parent" },
    ]);

    expect(el.textContent).toContain("Subagent");
    expect(el.textContent).toContain("Count Go files");
    // The subagent's tool is behind its own collapsed group, not in the main list.
    expect(el.textContent).not.toContain("find . -name");
  });

  it("renders nothing for an empty process list", () => {
    const el = render([], "");
    expect(el.textContent).toBe("");
  });

  it("still renders main-agent tools directly", () => {
    const el = render([
      { type: "tool_call", tool_name: "Bash", tool_call_id: "toolu_main", input: { command: "ls" } },
    ]);
    expect(el.textContent).toContain("Bash");
    expect(el.textContent).not.toContain("Subagent");
  });
});
