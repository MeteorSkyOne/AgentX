import type { Agent, ConversationAgentContext } from "../../api/types";

export function uniqueAgents(agents: Agent[]): Agent[] {
  const seen = new Set<string>();
  const result: Agent[] = [];
  for (const agent of agents) {
    if (!agent.id || seen.has(agent.id)) continue;
    seen.add(agent.id);
    result.push(agent);
  }
  return result;
}

export function uniqueConversationAgents(agents: ConversationAgentContext[]): ConversationAgentContext[] {
  const seen = new Set<string>();
  const result: ConversationAgentContext[] = [];
  for (const item of agents) {
    const id = item.agent.id || item.binding.agent_id;
    if (!id || seen.has(id)) continue;
    seen.add(id);
    result.push(item);
  }
  return result;
}
