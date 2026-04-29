import { Bot } from "lucide-react";
import { cn } from "@/lib/utils";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

const STORAGE_KEY = "agentx.agent_avatars";

export interface AgentAvatarData {
  emoji: string;
  color: string;
}

export const AVATAR_COLORS = [
  "bg-primary",
  "bg-[oklch(0.55_0.2_250)]",
  "bg-[oklch(0.6_0.2_145)]",
  "bg-[oklch(0.65_0.15_50)]",
  "bg-[oklch(0.55_0.22_25)]",
  "bg-[oklch(0.55_0.2_300)]",
  "bg-[oklch(0.6_0.15_180)]",
  "bg-[oklch(0.5_0.15_30)]",
];

export function getAgentAvatar(agentID: string): AgentAvatarData | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const map = JSON.parse(raw) as Record<string, AgentAvatarData>;
    return map[agentID] ?? null;
  } catch {
    return null;
  }
}

export function setAgentAvatar(agentID: string, data: AgentAvatarData | null): void {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    const map: Record<string, AgentAvatarData> = raw ? JSON.parse(raw) : {};
    if (data) {
      map[agentID] = data;
    } else {
      delete map[agentID];
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    // ignore
  }
}

export function agentKindColor(kind: string): string {
  if (kind === "codex" || kind === "codex-persistent")
    return "bg-[oklch(0.6_0.2_145)]";
  if (kind === "claude" || kind === "claude-persistent")
    return "bg-[oklch(0.65_0.15_50)]";
  return "bg-primary";
}

interface AgentAvatarProps {
  agentID: string;
  kind: string;
  size?: "sm" | "md" | "lg";
  className?: string;
}

const sizeMap = {
  sm: { avatar: "h-8 w-8", icon: "h-4 w-4", text: "text-base" },
  md: { avatar: "h-10 w-10", icon: "h-5 w-5", text: "text-lg" },
  lg: { avatar: "h-12 w-12", icon: "h-6 w-6", text: "text-xl" },
};

export function AgentAvatar({ agentID, kind, size = "md", className }: AgentAvatarProps) {
  const custom = getAgentAvatar(agentID);
  const s = sizeMap[size];

  if (custom?.emoji) {
    return (
      <Avatar className={cn(s.avatar, className)}>
        <AvatarFallback className={cn("text-white", custom.color || agentKindColor(kind))}>
          <span className={s.text}>{custom.emoji}</span>
        </AvatarFallback>
      </Avatar>
    );
  }

  return (
    <Avatar className={cn(s.avatar, className)}>
      <AvatarFallback className={cn("text-white", agentKindColor(kind))}>
        <Bot className={s.icon} />
      </AvatarFallback>
    </Avatar>
  );
}
