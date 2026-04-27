import { useEffect, useState } from "react";
import { Save, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select } from "@/components/ui/select";
import type { Agent, Channel, ConversationAgentContext, Workspace } from "../../api/types";
import { AgentAvatar } from "../AgentAvatar";
import type { ShellProps } from "./types";
import { agentKindLabel, agentToneColor, runWorkspaceOptions } from "./utils";

export function MembersPanel({
  agents,
  boundAgents,
  projectWorkspace,
  selectedChannel,
  onSaveChannelAgents,
  onClose,
}: {
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  projectWorkspace?: Workspace;
  selectedChannel?: Channel;
  onSaveChannelAgents: ShellProps["onSaveChannelAgents"];
  onClose: () => void;
}) {
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [runWorkspaceIDs, setRunWorkspaceIDs] = useState<Record<string, string>>({});
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const next: Record<string, boolean> = {};
    const nextRunWorkspaces: Record<string, string> = {};
    for (const item of boundAgents) {
      next[item.agent.id] = true;
      nextRunWorkspaces[item.agent.id] = item.binding.run_workspace_id ?? "";
    }
    setChecked(next);
    setRunWorkspaceIDs(nextRunWorkspaces);
    setDirty(false);
  }, [boundAgents]);

  function toggle(agentID: string, value: boolean) {
    setChecked((prev) => ({ ...prev, [agentID]: value }));
    setDirty(true);
  }

  async function save() {
    setSaving(true);
    try {
      const bindings = agents
        .filter((a) => checked[a.id])
        .map((a) => ({
          agent_id: a.id,
          run_workspace_id: runWorkspaceIDs[a.id]?.trim() || undefined,
        }));
      await onSaveChannelAgents(bindings);
      setDirty(false);
    } finally {
      setSaving(false);
    }
  }

  const bound = agents.filter((a) => checked[a.id]);
  const unbound = agents.filter((a) => !checked[a.id]);

  return (
    <aside className="flex h-full min-h-0 min-w-0 flex-col border-l border-border bg-card" aria-label="Channel members">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
        <span className="text-sm font-semibold">Members</span>
        <Button variant="ghost" size="icon" className="h-8 w-8" title="Close" aria-label="Close members" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3 space-y-4">
          {selectedChannel && (
            <p className="text-xs text-muted-foreground uppercase font-semibold px-1">
              #{selectedChannel.name}
            </p>
          )}

          {bound.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground uppercase font-semibold px-1">
                Bound — {bound.length}
              </p>
              {bound.map((a) => (
                <div
                  key={a.id}
                  className="picker-row rounded-md px-2 py-2 hover:bg-accent/50"
                >
                  <div className="flex items-center gap-2.5">
                    <Checkbox
                      checked
                      onChange={() => toggle(a.id, false)}
                    />
                    <div className="relative">
                      <AgentAvatar agentID={a.id} kind={a.kind} size="sm" />
                      <div className={cn(
                        "absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-card",
                        a.enabled ? "bg-green-500" : "bg-gray-500"
                      )} />
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{a.name}</p>
                      <p className="truncate text-xs text-muted-foreground">@{a.handle}</p>
                    </div>
                    <Badge variant="outline" className={cn("shrink-0 text-[10px]", agentToneColor(a.kind))}>
                      {agentKindLabel(a.kind)}
                    </Badge>
                  </div>
                  <Select
                    className="mt-2"
                    value={runWorkspaceIDs[a.id] ?? ""}
                    onChange={(e) => {
                      setRunWorkspaceIDs((current) => ({ ...current, [a.id]: e.target.value }));
                      setDirty(true);
                    }}
                    aria-label={`${a.name} run workspace`}
                    selectClassName="h-8 px-2 pr-8 text-xs"
                  >
                    {runWorkspaceOptions(a, boundAgents, projectWorkspace).map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </Select>
                </div>
              ))}
            </div>
          )}

          {unbound.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground uppercase font-semibold px-1">
                Available — {unbound.length}
              </p>
              {unbound.map((a) => (
                <label
                  key={a.id}
                  className="picker-row flex items-center gap-2.5 rounded-md px-2 py-2 hover:bg-accent/50 cursor-pointer opacity-60 hover:opacity-100 transition-opacity"
                >
                  <Checkbox
                    checked={false}
                    onChange={() => toggle(a.id, true)}
                  />
                  <AgentAvatar agentID={a.id} kind={a.kind} size="sm" />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{a.name}</p>
                    <p className="text-xs text-muted-foreground truncate">@{a.handle}</p>
                  </div>
                </label>
              ))}
            </div>
          )}

          {agents.length === 0 && (
            <p className="text-sm text-muted-foreground text-center py-4">No agents</p>
          )}
        </div>
      </ScrollArea>

      {dirty && (
        <div className="shrink-0 border-t border-border p-3">
          <Button size="sm" className="w-full gap-2" onClick={save} disabled={saving}>
            <Save className="h-4 w-4" />
            Save
          </Button>
        </div>
      )}
    </aside>
  );
}
