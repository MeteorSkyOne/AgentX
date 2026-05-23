import type { Agent, ConversationAgentSkills, ConversationSkill } from "../api/types";

export interface SlashCommandDefinition {
  kind: "command" | "skill";
  name: string;
  description: string;
  agentID?: string;
  agentHandle?: string;
  agentName?: string;
}

export interface TokenRange {
  start: number;
  end: number;
  query: string;
}

export interface CommandIndicator {
  status: "recognized" | "pending" | "unknown";
  label: string;
  title: string;
}

export const builtinSlashCommands: SlashCommandDefinition[] = [
  { kind: "command", name: "new", description: "Start fresh context" },
  { kind: "command", name: "skills", description: "List available skills" },
  { kind: "command", name: "compact", description: "Compact Claude context" },
  { kind: "command", name: "goal", description: "Set an autonomous goal" },
  { kind: "command", name: "status", description: "Show runtime status" },
  { kind: "command", name: "plan", description: "Ask for an implementation plan" },
  { kind: "command", name: "init", description: "Initialize agent instructions" },
  { kind: "command", name: "model", description: "Set the agent model" },
  { kind: "command", name: "effort", description: "Set reasoning effort" },
  { kind: "command", name: "commit", description: "Commit workspace changes" },
  { kind: "command", name: "push", description: "Push the current branch" },
  { kind: "command", name: "review", description: "Review workspace changes" },
  { kind: "command", name: "stop", description: "Stop active agent runs" },
  { kind: "command", name: "cancel", description: "Cancel active agent streams" },
  { kind: "command", name: "discuss", description: "Start a multi-agent discussion" }
];

export function buildSlashCommandOptions(
  groups: ConversationAgentSkills[],
  includeAgentLabel: boolean
): SlashCommandDefinition[] {
  const dynamic = groups.flatMap((group) =>
    group.skills
      .filter((skill) => !skill.conflicts_with_builtin)
      .map((skill) => skillCommandOption(group, skill, includeAgentLabel))
  );
  return [...builtinSlashCommands, ...dynamic];
}

function skillCommandOption(
  group: ConversationAgentSkills,
  skill: ConversationSkill,
  includeAgentLabel: boolean
): SlashCommandDefinition {
  const label = includeAgentLabel ? ` for @${group.agent_handle}` : "";
  return {
    kind: "skill",
    name: skill.name,
    description: skill.description || `Skill${label}`,
    agentID: group.agent_id,
    agentHandle: group.agent_handle,
    agentName: group.agent_name
  };
}

export function slashCommandKey(command: SlashCommandDefinition): string {
  return `${command.kind}:${command.agentID ?? ""}:${command.name}`;
}

export function commandLookupKey(value: string): string {
  return value.toLowerCase().replaceAll("_", "-");
}

export function slashCommandIndicator(
  value: string,
  commands: SlashCommandDefinition[]
): CommandIndicator | null {
  if (!value.startsWith("/")) return null;
  const token = value.split(/\s+/, 1)[0] ?? "";
  const name = token.slice(1).toLowerCase();
  return slashCommandIndicatorForName(name, commands);
}

export function slashCommandIndicatorForName(
  name: string,
  commands: SlashCommandDefinition[]
): CommandIndicator | null {
  if (!name) {
    return { status: "pending", label: "Command", title: "Slash command" };
  }
  const lookupName = commandLookupKey(name);
  if (commands.some((command) => commandLookupKey(command.name) === lookupName)) {
    return { status: "recognized", label: `/${name}`, title: `Recognized slash command /${name}` };
  }
  if (commands.some((command) => commandLookupKey(command.name).startsWith(lookupName))) {
    return { status: "pending", label: `/${name}`, title: "Partial slash command" };
  }
  return { status: "unknown", label: "Unknown", title: `Unknown slash command /${name}` };
}

export function slashCommandTokenAt(value: string, caret: number): TokenRange | null {
  if (caret < 0) return null;
  const beforeCaret = value.slice(0, caret);
  const match = /(^|\s)\/([A-Za-z0-9_-]*)$/.exec(beforeCaret);
  if (!match) return null;
  const prefix = match[1] ?? "";
  const start = match.index + prefix.length;
  let end = caret;
  while (end < value.length && !/\s/.test(value[end])) {
    end++;
  }
  return { start, end, query: value.slice(start + 1, end) };
}

export function mentionTokenAt(value: string, caret: number): TokenRange | null {
  if (caret < 0) return null;
  const beforeCaret = value.slice(0, caret);
  const match = /(^|[\s([{])@([A-Za-z0-9_-]*)$/.exec(beforeCaret);
  if (!match) return null;
  const prefix = match[1] ?? "";
  const start = match.index + prefix.length;
  return { start, end: caret, query: match[2] ?? "" };
}

export function mentionDisplayName(agent: Pick<Agent, "name" | "handle">): string {
  return agent.name.trim() || agent.handle;
}

export function mentionDisplayNamesToHandles(
  value: string,
  agents: Pick<Agent, "name" | "handle">[]
): string {
  const mentions = agents
    .map((agent) => ({ label: mentionDisplayName(agent), handle: agent.handle }))
    .filter((mention) => mention.label && mention.handle)
    .sort((a, b) => b.label.length - a.label.length);
  if (mentions.length === 0) return value;

  let result = "";
  let index = 0;
  while (index < value.length) {
    if (value[index] !== "@" || !isMentionBoundaryBefore(value, index)) {
      result += value[index];
      index += 1;
      continue;
    }
    const mention = mentions.find((candidate) => {
      const start = index + 1;
      const end = start + candidate.label.length;
      return (
        value.startsWith(candidate.label, start) && isMentionBoundaryAfter(value, end)
      );
    });
    if (!mention) {
      result += value[index];
      index += 1;
      continue;
    }
    result += `@${mention.handle}`;
    index += mention.label.length + 1;
  }
  return result;
}

function isMentionBoundaryBefore(value: string, index: number): boolean {
  if (index === 0) return true;
  return /[\s([{]/.test(value[index - 1]);
}

function isMentionBoundaryAfter(value: string, index: number): boolean {
  if (index >= value.length) return true;
  return !/[A-Za-z0-9_-]/.test(value[index]);
}
