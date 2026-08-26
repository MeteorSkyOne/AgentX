import { useEffect, useMemo, useState } from "react";
import { Save, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select } from "@/components/ui/select";
import type { Agent, Channel, ConversationAgentContext, Thread, Workspace } from "../../api/types";
import { AgentAvatar } from "../AgentAvatar";
import type { ShellProps } from "./types";
import { agentKindLabel, agentToneColor, runWorkspaceOptions } from "./utils";

export function MembersPanel({
  agents,
  boundAgents,
  channelAgents,
  projectWorkspace,
  selectedChannel,
  activeThread,
  onSaveChannelAgents,
  onSaveThreadAgents,
  onClose,
}: {
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  channelAgents?: ConversationAgentContext[];
  projectWorkspace?: Workspace;
  selectedChannel?: Channel;
  activeThread?: Thread;
  onSaveChannelAgents: ShellProps["onSaveChannelAgents"];
  onSaveThreadAgents?: ShellProps["onSaveThreadAgents"];
  onClose: () => void;
}) {
  // Inside a forum post the panel edits post membership, which can only be
  // picked from the agents already bound to the channel.
  const threadMode = Boolean(activeThread && onSaveThreadAgents);
  const candidates = useMemo(
    () => (threadMode ? (channelAgents ?? []).map((item) => item.agent) : agents),
    [agents, channelAgents, threadMode]
  );
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [runWorkspaceIDs, setRunWorkspaceIDs] = useState<Record<string, string>>({});
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
    setError(null);
  }, [boundAgents]);

  function toggle(agentID: string, value: boolean) {
    setChecked((prev) => ({ ...prev, [agentID]: value }));
    setDirty(true);
    setError(null);
  }

  async function save() {
    setSaving(true);
    setError(null);
    try {
      if (threadMode) {
        await onSaveThreadAgents!(candidates.filter((a) => checked[a.id]).map((a) => a.id));
      } else {
        const bindings = candidates
          .filter((a) => checked[a.id])
          .map((a) => ({
            agent_id: a.id,
            run_workspace_id: runWorkspaceIDs[a.id]?.trim() || undefined,
          }));
        await onSaveChannelAgents(bindings);
      }
      setDirty(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save members failed");
    } finally {
      setSaving(false);
    }
  }

  const bound = candidates.filter((a) => checked[a.id]);
  const unbound = candidates.filter((a) => !checked[a.id]);
  const saveDisabled = saving || (threadMode && bound.length === 0);

  return (
    <aside
      className="flex h-full min-h-0 min-w-0 flex-col border-l border-border bg-card"
      aria-label={threadMode ? "Post members" : "Channel members"}
    >
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
        <span className="text-sm font-semibold">Members</span>
        <Button variant="ghost" size="icon" className="h-8 w-8" title="Close" aria-label="Close members" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3 space-y-4">
          {threadMode ? (
            activeThread && (
              <p className="truncate px-1 text-xs font-semibold uppercase text-muted-foreground">
                {activeThread.title}
              </p>
            )
          ) : (
            selectedChannel && (
              <p className="text-xs text-muted-foreground uppercase font-semibold px-1">
                #{selectedChannel.name}
              </p>
            )
          )}

          {bound.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground uppercase font-semibold px-1">
                {threadMode ? "In this post" : "Bound"} — {bound.length}
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
                  {!threadMode && (
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
                  )}
                </div>
              ))}
            </div>
          )}

          {unbound.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground uppercase font-semibold px-1">
                {threadMode ? "Not in this post" : "Available"} — {unbound.length}
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

          {candidates.length === 0 && (
            <p className="text-sm text-muted-foreground text-center py-4">
              {threadMode ? "No agents bound to this channel" : "No agents"}
            </p>
          )}
        </div>
      </ScrollArea>

      {(dirty || error) && (
        <div className="shrink-0 space-y-2 border-t border-border p-3">
          {error && <p className="text-xs text-destructive">{error}</p>}
          {threadMode && bound.length === 0 && (
            <p className="text-xs text-muted-foreground">A post needs at least one member.</p>
          )}
          {dirty && (
            <Button size="sm" className="w-full gap-2" onClick={save} disabled={saveDisabled}>
              <Save className="h-4 w-4" />
              Save
            </Button>
          )}
        </div>
      )}
    </aside>
  );
}
