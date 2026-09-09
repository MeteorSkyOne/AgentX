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

function render(
  process: ProcessItem[],
  text = "done",
  onStopSubagent?: (toolCallID: string) => Promise<void>
) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => {
    root!.render(
      createElement(InterleavedMessageBody, {
        text,
        processItems: process,
        defaultProcessOpen: true,
        onStopSubagent,
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

  it("offers a stop button for a running subagent and reports its tool call", () => {
    const stopped: string[] = [];
    const onStop = (toolCallID: string) => {
      stopped.push(toolCallID);
      return Promise.resolve();
    };
    const el = render(
      [
        {
          type: "tool_call",
          tool_name: "Task",
          tool_call_id: "toolu_bg",
          input: { description: "Deep dive" },
        },
        { type: "subagent_started", tool_call_id: "toolu_bg" },
        { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_bg", input: {} },
      ],
      "body text",
      onStop
    );

    const button = el.querySelector('button[title="Stop subagent"]') as HTMLButtonElement | null;
    expect(button).not.toBeNull();
    act(() => {
      button!.click();
    });
    expect(stopped).toEqual(["toolu_bg"]);
    expect(button!.textContent).toContain("stopping");
  });

  it("hides the stop button once the subagent is done", () => {
    const el = render(
      [
        {
          type: "tool_call",
          tool_name: "Task",
          tool_call_id: "toolu_bg",
          input: { description: "Deep dive" },
        },
        { type: "subagent_started", tool_call_id: "toolu_bg" },
        { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_bg", input: {} },
        { type: "subagent_completed", tool_call_id: "toolu_bg", status: "completed" },
      ],
      "body text",
      () => Promise.resolve()
    );

    expect(el.querySelector('button[title="Stop subagent"]')).toBeNull();
  });

  it("renders nothing for an empty process list", () => {
    const el = render([], "");
    expect(el.textContent).toBe("");
  });

  it("keeps one subagent group with its description when process breaks split the timeline", () => {
    // Break markers count every process item (subagent work included); the
    // subagent's items span the break, but its group must render once, in the
    // block holding the spawning call.
    const el = render(
      [
        {
          type: "tool_call",
          tool_name: "Task",
          tool_call_id: "toolu_parent",
          input: { description: "Count Go files" },
        },
        {
          type: "tool_call",
          tool_name: "Bash",
          tool_call_id: "toolu_c1",
          parent_tool_call_id: "toolu_parent",
          input: { command: "echo one" },
        },
        {
          type: "tool_call",
          tool_name: "Bash",
          tool_call_id: "toolu_c2",
          parent_tool_call_id: "toolu_parent",
          input: { command: "echo two" },
        },
        { type: "tool_result", tool_call_id: "toolu_parent" },
      ],
      "before\n\n<!-- process-break:2 -->\n\nafter"
    );

    expect(el.textContent?.match(/Subagent/g)).toHaveLength(1);
    expect(el.textContent).toContain("Count Go files");
  });

  it("shows a subagent's real state from lifecycle signals", () => {
    const base: ProcessItem[] = [
      {
        type: "tool_call",
        tool_name: "Task",
        tool_call_id: "toolu_bg",
        input: { description: "Deep dive" },
      },
      { type: "tool_result", tool_call_id: "toolu_bg" },
      { type: "subagent_started", tool_call_id: "toolu_bg" },
      { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_bg", input: {} },
    ];

    const runningEl = render(base, "body text");
    expect(runningEl.textContent).toContain("running");
    expect(runningEl.textContent).not.toContain("done");

    act(() => {
      root?.unmount();
    });
    container?.remove();

    const doneEl = render(
      [...base, { type: "subagent_completed", tool_call_id: "toolu_bg", status: "completed" }],
      "body text"
    );
    expect(doneEl.textContent).toContain("done");
    expect(doneEl.textContent).not.toContain("running");
  });

  it("still renders main-agent tools directly", () => {
    const el = render([
      { type: "tool_call", tool_name: "Bash", tool_call_id: "toolu_main", input: { command: "ls" } },
    ]);
    expect(el.textContent).toContain("Bash");
    expect(el.textContent).not.toContain("Subagent");
  });

  it("names the skill an agent invoked instead of showing a generic tool call", () => {
    const el = render([
      {
        type: "tool_call",
        tool_name: "Skill",
        tool_call_id: "toolu_skill",
        input: { skill: "code-review", args: "--fix" },
      },
      { type: "tool_result", tool_call_id: "toolu_skill", output: "loaded" },
    ]);

    // The collapsed tools summary already names the skill.
    expect(el.textContent).toContain("/code-review");

    const trigger = Array.from(el.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("1 tool")
    );
    expect(trigger).toBeDefined();
    act(() => {
      trigger!.click();
    });

    expect(el.textContent).toContain("Skill");
    expect(el.textContent).toContain("/code-review");
    expect(el.textContent).toContain("--fix");
    expect(el.textContent).not.toContain("Tool call");
  });

  it("names invoked skills in the collapsed tools summary", () => {
    const el = render([
      { type: "tool_call", tool_name: "Skill", tool_call_id: "s1", input: { skill: "pdf" } },
      { type: "tool_result", tool_call_id: "s1" },
      { type: "tool_call", tool_name: "Read", tool_call_id: "r1", input: { file_path: "a.md" } },
      { type: "tool_result", tool_call_id: "r1" },
    ]);

    expect(el.textContent).toContain("2 tools");
    expect(el.textContent).toContain("/pdf, Read");
  });
});
