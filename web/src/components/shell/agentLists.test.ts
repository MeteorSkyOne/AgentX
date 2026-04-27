import { describe, expect, it } from "vitest";
import type { Agent, ConversationAgentContext } from "../../api/types";
import { uniqueAgents, uniqueConversationAgents } from "./agentLists";

describe("agent list helpers", () => {
  it("deduplicates organization agents by id", () => {
    const first = agent("agt_1", "Planner");
    const duplicate = agent("agt_1", "Planner duplicate");
    const second = agent("agt_2", "Reviewer");

    expect(uniqueAgents([first, duplicate, second])).toEqual([first, second]);
  });

  it("deduplicates conversation agent contexts by agent id", () => {
    const first = conversationAgent("agt_1", "Planner");
    const duplicate = conversationAgent("agt_1", "Planner duplicate");
    const second = conversationAgent("agt_2", "Reviewer");

    expect(uniqueConversationAgents([first, duplicate, second])).toEqual([first, second]);
  });
});

function agent(id: string, name: string): Agent {
  return {
    id,
    organization_id: "org_1",
    bot_user_id: `${id}_bot`,
    kind: "codex",
    name,
    handle: name.toLowerCase(),
    description: "",
    model: "",
    effort: "",
    config_workspace_id: "wks_config",
    default_workspace_id: "wks_default",
    enabled: true,
    fast_mode: false,
    yolo_mode: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function conversationAgent(id: string, name: string): ConversationAgentContext {
  return {
    binding: {
      channel_id: "chn_1",
      agent_id: id,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    },
    agent: agent(id, name),
    config_workspace: workspace("wks_config"),
    run_workspace: workspace("wks_run"),
  };
}

function workspace(id: string) {
  return {
    id,
    organization_id: "org_1",
    type: "agent",
    name: id,
    path: `/tmp/${id}`,
    created_by: "usr_1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}
