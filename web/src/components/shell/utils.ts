import type { Agent, Channel, ConversationAgentContext, Thread, Workspace } from "../../api/types";
import type { BrowserNotificationPermission } from "../../notifications/browser";

export const AGENT_EFFORT_OPTIONS = ["low", "medium", "high", "xhigh"] as const;

export function runWorkspaceOptions(
  agent: Agent,
  boundAgents: ConversationAgentContext[],
  projectWorkspace?: Workspace
): Array<{ value: string; label: string }> {
  const bound = boundAgents.find((item) => item.agent.id === agent.id);
  const currentOverride = bound?.binding.run_workspace_id ?? "";
  const ownWorkspaceID = agent.config_workspace_id || agent.default_workspace_id;
  const options = [
    {
      value: "",
      label: projectWorkspace?.path
        ? `Project workspace - ${projectWorkspace.path}`
        : "Project workspace",
    },
  ];

  if (ownWorkspaceID) {
    options.push({
      value: ownWorkspaceID,
      label: bound?.config_workspace.path
        ? `Agent workspace - ${bound.config_workspace.path}`
        : "Agent workspace",
    });
  }

  if (currentOverride && currentOverride !== ownWorkspaceID) {
    const isProjectWorkspace = currentOverride === projectWorkspace?.id;
    options.push({
      value: currentOverride,
      label: isProjectWorkspace
        ? "Project workspace (pinned)"
        : `Custom workspace - ${bound?.run_workspace.path ?? currentOverride}`,
    });
  }

  return options;
}

export function defaultAgentInstructionPath(kind?: string): string {
  return kind === "claude" ? "CLAUDE.md" : "AGENTS.md";
}

export function blurActiveElement() {
  const active = document.activeElement;
  if (active instanceof HTMLElement) {
    active.blur();
  }
}

export function browserPermissionLabel(permission: BrowserNotificationPermission): string {
  switch (permission) {
    case "granted":
      return "Enabled";
    case "denied":
      return "Blocked";
    case "unsupported":
      return "Unsupported";
    default:
      return "Not enabled";
  }
}

const projectAvatarStorageKey = "agentx.project_avatars";

interface ProjectAvatarData {
  emoji: string;
  color: string;
}

export function getProjectAvatar(projectID: string): ProjectAvatarData | null {
  try {
    const raw = localStorage.getItem(projectAvatarStorageKey);
    if (!raw) return null;
    const map = JSON.parse(raw) as Record<string, ProjectAvatarData>;
    return map[projectID] ?? null;
  } catch {
    return null;
  }
}

export function setProjectAvatar(projectID: string, data: ProjectAvatarData | null): void {
  try {
    const raw = localStorage.getItem(projectAvatarStorageKey);
    const map: Record<string, ProjectAvatarData> = raw ? JSON.parse(raw) : {};
    if (data) {
      map[projectID] = data;
    } else {
      delete map[projectID];
    }
    localStorage.setItem(projectAvatarStorageKey, JSON.stringify(map));
  } catch {
    // ignore
  }
}

export function conversationTitle(
  channel: Channel | undefined,
  thread: Thread | undefined,
  agents: Agent[]
): string {
  if (thread) return thread.title;
  if (channel?.type === "thread") return channel.name;
  if (agents.length === 1) return agents[0].name;
  return channel?.name ?? "No channel";
}

export function conversationSubtitle(
  channel: Channel | undefined,
  thread: Thread | undefined,
  agentCount: number
): string {
  if (!channel) return "No channel selected";
  if (thread) return `#${channel.name} · ${agentCount} agents`;
  return `${channel.type === "thread" ? "forum" : "text"} · ${agentCount} agents`;
}

export function agentToneColor(kind: string): string {
  if (kind === "codex") return "text-[oklch(0.7_0.2_145)]";
  if (kind === "claude") return "text-[oklch(0.75_0.15_50)]";
  return "text-muted-foreground";
}

export function agentKindLabel(kind: string): string {
  switch (kind) {
    case "codex": return "Codex";
    case "claude": return "Claude Code";
    case "fake": return "Fake runtime";
    default: return kind || "Agent";
  }
}

export function initials(value: string): string {
  return (
    value
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() ?? "")
      .join("") || "AX"
  );
}

export function normalizeAgentHandle(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[\s.]+/g, "_")
    .replace(/[^a-z0-9_-]+/g, "")
    .replace(/[_-]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

export function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit"
  }).format(new Date(value));
}
