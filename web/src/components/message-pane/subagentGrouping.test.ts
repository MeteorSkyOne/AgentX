import { describe, expect, it } from "vitest";
import type { ProcessItem } from "@/api/types";
import { partitionSubagentItems } from "./ProcessTimeline";

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
});
