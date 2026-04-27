import { useEffect, useMemo, useState } from "react";
import {
  Bot,
  Database,
  FolderOpen,
  Key,
  Plus,
  Save,
  Settings,
  Trash2,
  UserRound,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import type { Agent, Channel, ConversationAgentContext, Workspace } from "../../api/types";
import type { ThemeMode } from "../../theme";
import {
  AgentAvatar,
  AVATAR_COLORS,
  agentKindColor,
  getAgentAvatar,
  setAgentAvatar,
} from "../AgentAvatar";
import { WorkspaceFileBrowser } from "../WorkspaceFileBrowser";
import type { ShellProps } from "./types";
import {
  agentKindLabel,
  agentToneColor,
  defaultAgentInstructionPath,
  runWorkspaceOptions,
} from "./utils";

export function AgentDetailsPanel({
  selectedChannel,
  projectWorkspace,
  agents,
  boundAgents,
  selectedAgent,
  onSaveChannelAgents,
  onUpdateAgent,
  onDeleteAgent,
  onLoadWorkspaceTree,
  onReadWorkspaceFile,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onCreateAgentModal,
  onClose,
  theme
}: {
  selectedChannel?: Channel;
  projectWorkspace?: Workspace;
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  selectedAgent?: Agent;
  onSaveChannelAgents: (
    bindings: Array<{ agent_id: string; run_workspace_id?: string }>
  ) => Promise<void>;
  onUpdateAgent: ShellProps["onUpdateAgent"];
  onDeleteAgent: ShellProps["onDeleteAgent"];
  onLoadWorkspaceTree: ShellProps["onLoadWorkspaceTree"];
  onReadWorkspaceFile: ShellProps["onReadWorkspaceFile"];
  onWriteWorkspaceFile: ShellProps["onWriteWorkspaceFile"];
  onDeleteWorkspaceFile: ShellProps["onDeleteWorkspaceFile"];
  onCreateAgentModal: () => void;
  onClose: () => void;
  theme: ThemeMode;
}) {
  const [checkedAgents, setCheckedAgents] = useState<Record<string, boolean>>({});
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [agentID, setAgentID] = useState(selectedAgent?.id ?? "");
  const [name, setName] = useState(selectedAgent?.name ?? "");
  const [description, setDescription] = useState(selectedAgent?.description ?? "");
  const [handle, setHandle] = useState(selectedAgent?.handle ?? "");
  const [kind, setKind] = useState(selectedAgent?.kind ?? "fake");
  const [model, setModel] = useState(selectedAgent?.model ?? "");
  const [effort, setEffort] = useState(selectedAgent?.effort ?? "");
  const [enabled, setEnabled] = useState(selectedAgent?.enabled ?? true);
  const [fastMode, setFastMode] = useState(selectedAgent?.fast_mode ?? false);
  const [yoloMode, setYoloMode] = useState(selectedAgent?.yolo_mode ?? false);
  const [avatarEmoji, setAvatarEmoji] = useState("");
  const [avatarColor, setAvatarColor] = useState("");
  const [activeTab, setActiveTab] = useState("settings");
  const [envBody, setEnvBody] = useState("{}");
  const [status, setStatus] = useState<string | null>(null);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const selected = agents.find((a) => a.id === agentID) ?? selectedAgent;
  const selectedBinding = boundAgents.find((item) => item.agent.id === selected?.id);
  const selectedConfigWorkspaceID = selected?.config_workspace_id ?? "";
  const envEntries = useMemo(
    () => Object.entries(selected?.env ?? {}).sort(([l], [r]) => l.localeCompare(r)),
    [selected?.env]
  );

  useEffect(() => {
    if (!selectedAgent?.id) return;
    setAgentID(selectedAgent.id);
    setActiveTab("settings");
  }, [selectedAgent?.id]);

  useEffect(() => {
    const nextChecked: Record<string, boolean> = {};
    const nextOverrides: Record<string, string> = {};
    for (const item of boundAgents) {
      nextChecked[item.agent.id] = true;
      nextOverrides[item.agent.id] = item.binding.run_workspace_id ?? "";
    }
    setCheckedAgents(nextChecked);
    setOverrides(nextOverrides);
  }, [boundAgents]);

  useEffect(() => {
    if (!selected) return;
    setAgentID(selected.id);
    setName(selected.name);
    setDescription(selected.description ?? "");
    setHandle(selected.handle);
    setKind(selected.kind);
    setModel(selected.model);
    setEffort(selected.effort ?? "");
    setEnabled(selected.enabled);
    setFastMode(selected.fast_mode);
    setYoloMode(selected.yolo_mode);
    setEnvBody("{}");
    const av = getAgentAvatar(selected.id);
    setAvatarEmoji(av?.emoji ?? "");
    setAvatarColor(av?.color ?? "");
    setDeleteConfirmOpen(false);
  }, [
    selected?.id,
    selected?.name,
    selected?.description,
    selected?.handle,
    selected?.kind,
    selected?.model,
    selected?.effort,
    selected?.enabled,
    selected?.fast_mode,
    selected?.yolo_mode
  ]);

  async function saveBindings() {
    const bindings = agents
      .filter((a) => checkedAgents[a.id])
      .map((a) => ({
        agent_id: a.id,
        run_workspace_id: overrides[a.id]?.trim() || undefined
      }));
    await onSaveChannelAgents(bindings);
    setStatus("Saved");
  }

  async function saveAgent() {
    if (!selected) return;
    await onUpdateAgent(selected.id, { name, description, handle, kind, model, effort, enabled, fast_mode: fastMode, yolo_mode: yoloMode });
    setAgentAvatar(selected.id, avatarEmoji ? { emoji: avatarEmoji, color: avatarColor || agentKindColor(kind) } : null);
    setStatus("Saved");
  }

  async function saveEnv() {
    if (!selected) return;
    try {
      const parsed = JSON.parse(envBody) as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        throw new Error("env must be an object");
      }
      const env: Record<string, string> = {};
      for (const [key, value] of Object.entries(parsed)) {
        env[key] = String(value);
      }
      await onUpdateAgent(selected.id, { env });
      setStatus("Saved");
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "Invalid JSON");
    }
  }

  async function confirmDeleteAgent() {
    if (!selected) return;
    setDeleting(true);
    try {
      await onDeleteAgent(selected.id);
      setAgentID("");
      setDeleteConfirmOpen(false);
      setStatus("Deleted");
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeleting(false);
    }
  }

  const agentColor = agentToneColor(selected?.kind ?? "fake");

  return (
    <>
    <aside className="flex h-full min-h-0 min-w-0 flex-col border-l border-border bg-card" aria-label="Agent details">
      {/* Header */}
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
        <div className="flex items-center gap-2">
          <Bot className={cn("h-5 w-5", agentColor)} />
          <span className="font-semibold">Agent Config</span>
        </div>
        <Button variant="ghost" size="icon" className="h-8 w-8" title="Close" aria-label="Close" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      {/* Agent Info */}
      <div className="shrink-0 border-b border-border p-4">
        <div className="flex items-center gap-3">
          {selected ? (
            <AgentAvatar agentID={selected.id} kind={selected.kind} size="lg" className="rounded-xl" />
          ) : (
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary">
              <Bot className="h-6 w-6 text-white" />
            </div>
          )}
          <div className="min-w-0 flex-1">
            <h2 className="truncate font-semibold">{selected?.name ?? "Agents"}</h2>
            <div className="flex items-center gap-2">
              <Badge variant="outline" className={cn("text-xs", agentColor)}>
                {agentKindLabel(selected?.kind ?? "fake")}
              </Badge>
                <span className="truncate text-xs text-muted-foreground">
                  {selected?.handle ?? ""}
                </span>
              </div>
              {selected?.description && (
                <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{selected.description}</p>
              )}
            </div>
          </div>
        <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <span className="text-muted-foreground">Model</span>
          <strong className="truncate">{selected?.model || "default"}</strong>
          <span className="text-muted-foreground">Effort</span>
          <strong className="truncate">{selected?.effort || "default"}</strong>
          <span className="text-muted-foreground">Fast</span>
          <strong>{selected?.fast_mode ? "on" : "off"}</strong>
          <span className="text-muted-foreground">YOLO</span>
          <strong>{selected?.yolo_mode ? "on" : "off"}</strong>
          <span className="text-muted-foreground">Channel</span>
          <strong className="truncate">{selectedChannel ? `#${selectedChannel.name}` : "none"}</strong>
        </div>
        <div className="workspace-path mt-2 flex items-center gap-1 text-xs text-muted-foreground">
          <Database className="h-3 w-3" />
          <span className="truncate">
            Run: {selectedBinding?.run_workspace.path ?? projectWorkspace?.path ?? "project workspace"}
          </span>
        </div>
        <div className="workspace-path mt-1 flex items-center gap-1 text-xs text-muted-foreground">
          <FolderOpen className="h-3 w-3" />
          <span className="truncate">
            Own: {selectedBinding?.config_workspace.path ?? selected?.config_workspace_id ?? ""}
          </span>
        </div>
        <div className="mt-1 text-xs text-muted-foreground">
          {envEntries.length > 0
            ? envEntries.map(([k]) => k).join(", ")
            : "empty"}
        </div>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <TabsList className="mx-3 mt-4 grid w-auto grid-cols-4 md:mx-4">
          <TabsTrigger value="settings" className="gap-1 text-xs">
            <Settings className="h-3.5 w-3.5" />
            Settings
          </TabsTrigger>
          <TabsTrigger value="members" className="gap-1 text-xs">
            <UserRound className="h-3.5 w-3.5" />
            Members
          </TabsTrigger>
          <TabsTrigger value="workspace" className="gap-1 text-xs">
            <FolderOpen className="h-3.5 w-3.5" />
            Files
          </TabsTrigger>
          <TabsTrigger value="env" className="gap-1 text-xs">
            <Key className="h-3.5 w-3.5" />
            Env
          </TabsTrigger>
        </TabsList>

        {/* Settings Tab */}
        <TabsContent value="settings" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
          <ScrollArea className="h-full">
            <div className="space-y-4 py-1 pl-1 pr-3">
              {/* Agent Selector */}
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground uppercase">Agent</Label>
                <Select
                  value={agentID}
                  onChange={(e) => setAgentID(e.target.value)}
                  aria-label="Agent"
                >
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>{a.name} (@{a.handle})</option>
                  ))}
                </Select>
              </div>

              {/* Edit Fields */}
              <div className="space-y-2">
                <Label className="text-xs">Name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} aria-label="Agent name" />
              </div>
              <div className="space-y-2">
                <Label className="text-xs">Description</Label>
                <Textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  aria-label="Agent description"
                  rows={3}
                />
              </div>
              <div className="space-y-2">
                <Label className="text-xs">Handle</Label>
                <Input value={handle} onChange={(e) => setHandle(e.target.value)} aria-label="Agent handle" />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-2">
                  <Label className="text-xs">Runtime</Label>
                  <Select
                    value={kind}
                    onChange={(e) => setKind(e.target.value)}
                    aria-label="Agent runtime"
                  >
                    <option value="fake">Fake</option>
                    <option value="codex">Codex</option>
                    <option value="claude">Claude</option>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label className="text-xs">Model</Label>
                  <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="Model" aria-label="Agent model" />
                </div>
              </div>
              <div className="space-y-2">
                <Label className="text-xs">Effort</Label>
                <Select
                  value={effort}
                  onChange={(e) => setEffort(e.target.value)}
                  aria-label="Agent effort"
                >
                  <option value="">Default</option>
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                </Select>
              </div>
              <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
                <Checkbox
                  checked={enabled}
                  onChange={(e) => setEnabled(e.target.checked)}
                />
                Enabled
              </label>
              <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
                <Checkbox
                  checked={fastMode}
                  onChange={(e) => setFastMode(e.target.checked)}
                  aria-label="Agent fast mode"
                />
                Fast mode
              </label>
              <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
                <Checkbox
                  checked={yoloMode}
                  onChange={(e) => setYoloMode(e.target.checked)}
                  aria-label="Agent YOLO mode"
                />
                YOLO mode
              </label>
              {/* Avatar */}
              <div className="space-y-2">
                <Label className="text-xs">Avatar</Label>
                <div className="flex items-center gap-3">
                  {selected && <AgentAvatar agentID={selected.id} kind={kind} size="md" />}
                  <Input
                    value={avatarEmoji}
                    onChange={(e) => setAvatarEmoji(e.target.value)}
                    placeholder="Emoji (e.g. 🤖)"
                    aria-label="Agent avatar"
                    className="flex-1"
                  />
                </div>
                <div className="flex gap-1.5 flex-wrap">
                  {AVATAR_COLORS.map((c) => (
                    <button
                      key={c}
                      className={cn(
                        "h-6 w-6 rounded-full transition-all",
                        c,
                        avatarColor === c ? "ring-2 ring-ring ring-offset-2 ring-offset-card" : "opacity-60 hover:opacity-100"
                      )}
                      onClick={() => setAvatarColor(c)}
                      type="button"
                    />
                  ))}
                  {avatarEmoji && (
                    <button
                      className="h-6 px-2 rounded-full text-xs text-muted-foreground hover:text-foreground border border-border"
                      onClick={() => { setAvatarEmoji(""); setAvatarColor(""); }}
                      type="button"
                    >
                      Reset
                    </button>
                  )}
                </div>
              </div>

              <Button size="sm" className="w-full gap-2" onClick={saveAgent}>
                <Save className="h-4 w-4" />
                Save
              </Button>

              <Button size="sm" variant="outline" className="w-full gap-2" onClick={onCreateAgentModal}>
                <Plus className="h-4 w-4" />
                Create new agent
              </Button>

              {selected && (
                <Button
                  size="sm"
                  variant="destructive"
                  className="w-full gap-2"
                  onClick={() => setDeleteConfirmOpen(true)}
                >
                  <Trash2 className="h-4 w-4" />
                  Delete agent
                </Button>
              )}
              {status && <p className="text-xs text-muted-foreground">{status}</p>}
            </div>
          </ScrollArea>
        </TabsContent>

        {/* Members Tab */}
        <TabsContent value="members" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
          <ScrollArea className="h-full">
            <div className="space-y-3 pr-2">
              <p className="text-xs text-muted-foreground">
                {selectedChannel ? `#${selectedChannel.name}` : "No channel selected"}
              </p>
              {agents.map((a) => (
                <div key={a.id} className="picker-row rounded-lg border border-border p-3">
                  <label className="flex items-center gap-2">
                    <Checkbox
                      checked={Boolean(checkedAgents[a.id])}
                      onChange={(e) =>
                        setCheckedAgents((c) => ({ ...c, [a.id]: e.target.checked }))
                      }
                    />
                    <span className="text-sm font-medium">{a.name}</span>
                  </label>
                  <Select
                    className="mt-2"
                    value={overrides[a.id] ?? ""}
                    onChange={(e) => setOverrides((c) => ({ ...c, [a.id]: e.target.value }))}
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
              <Button size="sm" className="w-full gap-2" onClick={saveBindings}>
                <Save className="h-4 w-4" />
                Save
              </Button>
            </div>
          </ScrollArea>
        </TabsContent>

        {/* Workspace Tab */}
        <TabsContent value="workspace" className="flex min-h-0 flex-1 flex-col overflow-hidden px-4 pb-4">
          <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden">
            <WorkspaceFileBrowser
              workspaceID={selectedConfigWorkspaceID}
              workspacePath={selectedBinding?.config_workspace.path ?? selected?.config_workspace_id ?? ""}
              initialPath={defaultAgentInstructionPath(selected?.kind)}
              theme={theme}
              onLoadTree={onLoadWorkspaceTree}
              onReadFile={onReadWorkspaceFile}
              onWriteFile={onWriteWorkspaceFile}
              onDeleteFile={onDeleteWorkspaceFile}
            />
          </div>
        </TabsContent>

        {/* Env Tab */}
        <TabsContent value="env" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
          <ScrollArea className="h-full">
            <div className="space-y-3 pr-2">
              <span className="text-xs font-medium text-muted-foreground uppercase">Current</span>
              {envEntries.length > 0 ? (
                envEntries.map(([key, value]) => (
                  <div key={key} className="flex items-center justify-between rounded-lg border border-border p-2 text-xs">
                    <span className="font-mono font-medium">{key}</span>
                    <code className="text-muted-foreground">{value}</code>
                  </div>
                ))
              ) : (
                <p className="text-xs text-muted-foreground">empty</p>
              )}

              <span className="text-xs font-medium text-muted-foreground uppercase">Update (JSON)</span>
              <Textarea
                className="resize-none font-mono text-xs"
                value={envBody}
                onChange={(e) => setEnvBody(e.target.value)}
                aria-label="Environment JSON"
                rows={4}
              />
              <Button size="sm" className="w-full gap-2" onClick={saveEnv}>
                <Save className="h-4 w-4" />
                Save env
              </Button>
              {status && <p className="text-xs text-muted-foreground">{status}</p>}
            </div>
          </ScrollArea>
        </TabsContent>
      </Tabs>
    </aside>
    <Dialog open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete agent?</DialogTitle>
          <DialogDescription>
            {selected
              ? `${selected.name} will be disabled and removed from active channel replies.`
              : "This agent will be disabled and removed from active channel replies."}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setDeleteConfirmOpen(false)} disabled={deleting}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={confirmDeleteAgent} disabled={deleting || !selected}>
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    </>
  );
}
