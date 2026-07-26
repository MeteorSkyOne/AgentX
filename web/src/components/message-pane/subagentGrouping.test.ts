import { describe, expect, it } from "vitest";
import type { ProcessItem } from "@/api/types";
import { getSubagentSummary, partitionSubagentItems } from "./ProcessTimeline";

const agentCall: ProcessItem = {
  type: "tool_call",
  tool_name: "Agent",
  tool_call_id: "toolu_parent",
  input: { description: "Count Go files", subagent_type: "Explore" },
};

describe("partitionSubagentItems", () => {
  it("keeps the main agent's own items in the main timeline", () => {
    const items: ProcessItem[] = [
      { type: "thinking", text: "planning" },
      agentCall,
      { type: "tool_result", tool_call_id: "toolu_parent" },
    ];

    const { mainItems, subagentGroups } = partitionSubagentItems(items);

    expect(mainItems).toHaveLength(3);
    expect(subagentGroups).toHaveLength(0);
  });

  it("groups a subagent's work under the call that spawned it", () => {
    const items: ProcessItem[] = [
      agentCall,
      { type: "tool_call", tool_name: "Bash", tool_call_id: "toolu_c1", parent_tool_call_id: "toolu_parent" },
      { type: "tool_result", tool_call_id: "toolu_c1", parent_tool_call_id: "toolu_parent" },
      { type: "tool_result", tool_call_id: "toolu_parent" },
    ];

    const { mainItems, subagentGroups } = partitionSubagentItems(items);

    expect(mainItems.map((item) => item.tool_call_id)).toEqual(["toolu_parent", "toolu_parent"]);
    expect(subagentGroups).toHaveLength(1);
    expect(subagentGroups[0].parentToolCallID).toBe("toolu_parent");
    expect(subagentGroups[0].items).toHaveLength(2);
    expect(subagentGroups[0].description).toBe("Count Go files");
  });

  it("separates concurrent subagents", () => {
    const items: ProcessItem[] = [
      agentCall,
      {
        type: "tool_call",
        tool_name: "Agent",
        tool_call_id: "toolu_other",
        input: { description: "Review tests" },
      },
      { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_parent" },
      { type: "tool_call", tool_name: "Read", parent_tool_call_id: "toolu_other" },
      { type: "tool_call", tool_name: "Grep", parent_tool_call_id: "toolu_parent" },
    ];

    const { subagentGroups } = partitionSubagentItems(items);

    expect(subagentGroups).toHaveLength(2);
    const byParent = Object.fromEntries(subagentGroups.map((g) => [g.parentToolCallID, g]));
    expect(byParent["toolu_parent"].items).toHaveLength(2);
    expect(byParent["toolu_other"].items).toHaveLength(1);
    expect(byParent["toolu_other"].description).toBe("Review tests");
  });

  it("falls back to a generic label when the spawning call is not in this message", () => {
    const items: ProcessItem[] = [
      { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_missing" },
    ];

    const { mainItems, subagentGroups } = partitionSubagentItems(items);

    expect(mainItems).toHaveLength(0);
    expect(subagentGroups).toHaveLength(1);
    expect(subagentGroups[0].description).toBe("Subagent");
  });

  it("maps break positions in the full list to positions among main items", () => {
    const items: ProcessItem[] = [
      agentCall,
      { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_parent" },
      { type: "tool_result", parent_tool_call_id: "toolu_parent" },
      { type: "tool_result", tool_call_id: "toolu_parent" },
    ];

    const { mainCounts } = partitionSubagentItems(items);

    expect(mainCounts).toEqual([0, 1, 1, 1, 2]);
  });

  it("consumes lifecycle signals for group state without rendering them", () => {
    const items: ProcessItem[] = [
      agentCall,
      { type: "subagent_started", tool_call_id: "toolu_parent" },
      { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_parent" },
      { type: "tool_result", tool_call_id: "toolu_parent" },
    ];

    const { mainItems, subagentGroups, mainCounts } = partitionSubagentItems(items);

    expect(mainItems).toHaveLength(2);
    expect(mainCounts).toEqual([0, 1, 1, 1, 2]);
    expect(subagentGroups[0].active).toBe(true);

    const done = partitionSubagentItems([
      ...items,
      { type: "subagent_completed", tool_call_id: "toolu_parent", status: "completed" },
    ]);
    expect(done.subagentGroups[0].active).toBe(false);
  });

  it("anchors a group at its spawning call so it renders in the right block", () => {
    const items: ProcessItem[] = [
      { type: "thinking", text: "planning" },
      agentCall,
      { type: "tool_call", tool_name: "Bash", parent_tool_call_id: "toolu_parent" },
    ];

    const { subagentGroups } = partitionSubagentItems(items);

    expect(subagentGroups[0].anchor).toBe(1);
  });
});

describe("getSubagentSummary", () => {
  it("counts Claude's Task calls as subagents", () => {
    const summary = getSubagentSummary([
      {
        type: "tool_call",
        tool_name: "Task",
        tool_call_id: "toolu_task",
        input: { description: "Research the runtime" },
      },
    ]);

    expect(summary).toHaveLength(1);
    expect(summary[0].description).toBe("Research the runtime");
    expect(summary[0].active).toBe(true);
  });

  it("counts Codex's Agent calls and marks resolved ones done", () => {
    const summary = getSubagentSummary([
      { type: "tool_call", tool_name: "Agent", tool_call_id: "toolu_a", input: { description: "Review" } },
      { type: "tool_result", tool_call_id: "toolu_a" },
    ]);

    expect(summary).toHaveLength(1);
    expect(summary[0].active).toBe(false);
  });

  it("ignores ordinary tool calls", () => {
    const summary = getSubagentSummary([
      { type: "tool_call", tool_name: "Bash", tool_call_id: "toolu_b", input: { command: "ls" } },
    ]);

    expect(summary).toHaveLength(0);
  });

  it("trusts lifecycle signals over the spawning call's early result", () => {
    // A background Task's tool_result arrives when the task launches, so the
    // signals decide when the subagent is actually done.
    const running: ProcessItem[] = [
      { type: "tool_call", tool_name: "Task", tool_call_id: "toolu_bg", input: { description: "Deep dive" } },
      { type: "tool_result", tool_call_id: "toolu_bg" },
      { type: "subagent_started", tool_call_id: "toolu_bg" },
    ];
    expect(getSubagentSummary(running)[0].active).toBe(true);

    const finished: ProcessItem[] = [
      ...running,
      { type: "subagent_completed", tool_call_id: "toolu_bg", status: "completed" },
    ];
    expect(getSubagentSummary(finished)[0].active).toBe(false);
  });
});
