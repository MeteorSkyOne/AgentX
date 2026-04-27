import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Bot,
  Database,
  FileText,
  FolderOpen,
  Hash,
  Key,
  Plus,
  RefreshCw,
  Save,
  Settings,
  Trash2,
  X,
} from "lucide-react";
import { agentChannels as fetchAgentChannels } from "../../api/client";
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
import type { Agent, Channel, ConversationAgentContext, Workspace, WorkspaceTreeEntry } from "../../api/types";
import {
  AgentAvatar,
  AVATAR_COLORS,
  agentKindColor,
  getAgentAvatar,
  setAgentAvatar,
} from "../AgentAvatar";
import { FileTree } from "../FileTree";
import type { ShellProps } from "./types";
import {
  agentKindLabel,
  agentToneColor,
  defaultAgentInstructionPath,
} from "./utils";

export function AgentDetailsPanel({
  selectedChannel,
  projectWorkspace,
  agents,
  boundAgents,
  selectedAgent,
  onUpdateAgent,
  onDeleteAgent,
  onLoadWorkspaceTree,
  onReadWorkspaceFile,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onCreateAgentModal,
  onClose
}: {
  selectedChannel?: Channel;
  projectWorkspace?: Workspace;
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  selectedAgent?: Agent;
  onUpdateAgent: ShellProps["onUpdateAgent"];
  onDeleteAgent: ShellProps["onDeleteAgent"];
  onLoadWorkspaceTree: ShellProps["onLoadWorkspaceTree"];
  onReadWorkspaceFile: ShellProps["onReadWorkspaceFile"];
  onWriteWorkspaceFile: ShellProps["onWriteWorkspaceFile"];
  onDeleteWorkspaceFile: ShellProps["onDeleteWorkspaceFile"];
  onCreateAgentModal: () => void;
  onClose: () => void;
}) {
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
  const [filePath, setFilePath] = useState(() => defaultAgentInstructionPath(selectedAgent?.kind));
  const [fileBody, setFileBody] = useState("");
  const [tree, setTree] = useState<WorkspaceTreeEntry>();
  const [workspaceTreeLoading, setWorkspaceTreeLoading] = useState(false);
  const [workspaceTreeError, setWorkspaceTreeError] = useState<string | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileSaving, setFileSaving] = useState(false);
  const [fileDeleteConfirmOpen, setFileDeleteConfirmOpen] = useState(false);
  const [fileDeleting, setFileDeleting] = useState(false);
  const [workspaceStatus, setWorkspaceStatus] = useState<string | null>(null);
  const [envBody, setEnvBody] = useState("{}");
  const [status, setStatus] = useState<string | null>(null);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const workspaceTreeRequestRef = useRef(0);
  const fileRequestRef = useRef(0);

  const selected = agents.find((a) => a.id === agentID) ?? selectedAgent;
  const selectedBinding = boundAgents.find((item) => item.agent.id === selected?.id);
  const selectedAgentID = selected?.id ?? "";
  const agentChannelsQuery = useQuery({
    queryKey: ["agent-channels", selectedAgentID],
    queryFn: () => fetchAgentChannels(selectedAgentID),
    enabled: Boolean(selectedAgentID),
  });
  const joinedChannels = agentChannelsQuery.data ?? [];
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
    setFilePath(defaultAgentInstructionPath(selected.kind));
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

  useEffect(() => {
    workspaceTreeRequestRef.current += 1;
    fileRequestRef.current += 1;
    setTree(undefined);
    setFileBody("");
    setWorkspaceTreeError(null);
    setWorkspaceTreeLoading(false);
    setFileLoading(false);
    setFileSaving(false);
    setFileDeleteConfirmOpen(false);
    setFileDeleting(false);
    setWorkspaceStatus(null);
  }, [selectedConfigWorkspaceID]);

  useEffect(() => {
    if (activeTab === "workspace" && selectedConfigWorkspaceID) {
      void loadTree({ quiet: true });
    }
  }, [activeTab, selectedConfigWorkspaceID]);

  async function saveAgent() {
    if (!selected) return;
    await onUpdateAgent(selected.id, { name, description, handle, kind, model, effort, enabled, fast_mode: fastMode, yolo_mode: yoloMode });
    setAgentAvatar(selected.id, avatarEmoji ? { emoji: avatarEmoji, color: avatarColor || agentKindColor(kind) } : null);
    setStatus("Saved");
  }

  async function loadFile(path = filePath) {
    const targetPath = path.trim();
    if (!selectedConfigWorkspaceID || !targetPath) return;
    const requestID = ++fileRequestRef.current;
    setFileLoading(true);
    setFilePath(targetPath);
    try {
      const body = await onReadWorkspaceFile(selectedConfigWorkspaceID, targetPath);
      if (fileRequestRef.current !== requestID) return;
      setFileBody(body);
      setWorkspaceStatus("Loaded");
    } catch (err) {
      if (fileRequestRef.current !== requestID) return;
      setWorkspaceStatus(err instanceof Error ? err.message : "Load failed");
    } finally {
      if (fileRequestRef.current === requestID) {
        setFileLoading(false);
      }
    }
  }

  async function loadTree(options: { quiet?: boolean } = {}) {
    if (!selectedConfigWorkspaceID) return;
    const requestID = ++workspaceTreeRequestRef.current;
    setWorkspaceTreeLoading(true);
    setWorkspaceTreeError(null);
    try {
      const nextTree = await onLoadWorkspaceTree(selectedConfigWorkspaceID);
      if (workspaceTreeRequestRef.current !== requestID) return;
      setTree(nextTree);
      if (!options.quiet) setWorkspaceStatus("Tree loaded");
    } catch (err) {
      if (workspaceTreeRequestRef.current !== requestID) return;
      const message = err instanceof Error ? err.message : "Tree load failed";
      setWorkspaceTreeError(message);
      if (!options.quiet) setWorkspaceStatus(message);
    } finally {
      if (workspaceTreeRequestRef.current === requestID) {
        setWorkspaceTreeLoading(false);
      }
    }
  }

  async function saveFile() {
    const targetPath = filePath.trim();
    if (!selectedConfigWorkspaceID || !targetPath) return;
    setFileSaving(true);
    try {
      await onWriteWorkspaceFile(selectedConfigWorkspaceID, targetPath, fileBody);
      setFilePath(targetPath);
      setWorkspaceStatus("Saved");
      await loadTree({ quiet: true });
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Save failed");
    } finally {
      setFileSaving(false);
    }
  }

  async function confirmDeleteFile() {
    const targetPath = filePath.trim();
    if (!selectedConfigWorkspaceID || !targetPath) return;
    setFileDeleting(true);
    try {
      await onDeleteWorkspaceFile(selectedConfigWorkspaceID, targetPath);
      setFileBody("");
      setWorkspaceStatus("Deleted");
      setFileDeleteConfirmOpen(false);
      await loadTree({ quiet: true });
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setFileDeleting(false);
    }
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
          <TabsTrigger value="channels" className="gap-1 text-xs">
            <Hash className="h-3.5 w-3.5" />
            Channels
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

        {/* Channels Tab */}
        <TabsContent value="channels" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
          <ScrollArea className="h-full">
            <div className="space-y-3 pr-2">
              {agentChannelsQuery.isLoading ? (
                <p className="text-xs text-muted-foreground">Loading channels...</p>
              ) : agentChannelsQuery.isError ? (
                <p className="text-xs text-muted-foreground">
                  {agentChannelsQuery.error instanceof Error
                    ? agentChannelsQuery.error.message
                    : "Load channels failed"}
                </p>
              ) : joinedChannels.length === 0 ? (
                <p className="text-xs text-muted-foreground">No joined channels</p>
              ) : (
                joinedChannels.map((item) => (
                  <div
                    key={item.channel.id}
                    className={cn(
                      "rounded-lg border border-border p-3",
                      item.channel.id === selectedChannel?.id && "border-primary/50 bg-accent/40"
                    )}
                  >
                    <div className="flex min-w-0 items-center gap-2">
                      <Hash className="h-4 w-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate text-sm font-medium">{item.channel.name}</span>
                      <Badge variant="outline" className="shrink-0 text-[10px] uppercase">
                        {item.channel.type}
                      </Badge>
                    </div>
                    <p className="mt-1 truncate text-xs text-muted-foreground">{item.project.name}</p>
                    <div className="workspace-path mt-2 flex items-center gap-1 text-xs text-muted-foreground">
                      <Database className="h-3 w-3" />
                      <span className="truncate">Run: {item.run_workspace.path}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </ScrollArea>
        </TabsContent>

        {/* Workspace Tab */}
        <TabsContent value="workspace" className="flex min-h-0 flex-1 flex-col overflow-hidden px-4 pb-4">
          <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden">
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Database className="h-3.5 w-3.5" />
              <span className="truncate">{selectedBinding?.config_workspace.path ?? selected?.config_workspace_id ?? ""}</span>
            </div>
            <div className="flex gap-2">
              <Input value={filePath} onChange={(e) => setFilePath(e.target.value)} aria-label="File path" className="flex-1 text-xs" />
              <Button
                size="sm"
                variant="outline"
                className="gap-1.5"
                onClick={() => void loadFile()}
                disabled={fileLoading || !selectedConfigWorkspaceID || !filePath.trim()}
              >
                <FileText className="h-3.5 w-3.5" />
                Open
              </Button>
              <Button
                size="icon-sm"
                variant="outline"
                onClick={() => void loadTree()}
                disabled={workspaceTreeLoading || !selectedConfigWorkspaceID}
                title="Refresh tree"
                aria-label="Refresh tree"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", workspaceTreeLoading && "animate-spin")} />
              </Button>
            </div>
            <FileTree
              tree={tree}
              selectedPath={filePath}
              loading={workspaceTreeLoading}
              error={workspaceTreeError}
              className="h-44 shrink-0"
              ariaLabel="Agent workspace files"
              onSelectFile={(path) => void loadFile(path)}
            />
              <Textarea
                className="flex-1 resize-none font-mono text-xs"
                value={fileBody}
                onChange={(e) => setFileBody(e.target.value)}
                placeholder="File content..."
                aria-label="File content"
              />
              {workspaceStatus && <p className="text-xs text-muted-foreground">{workspaceStatus}</p>}
              <div className="grid grid-cols-2 gap-2">
                <Button
                  size="sm"
                  className="gap-2"
                  onClick={saveFile}
                  disabled={fileSaving || !selectedConfigWorkspaceID || !filePath.trim()}
                >
                  <Save className="h-4 w-4" />
                  Save file
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  className="gap-2"
                  onClick={() => setFileDeleteConfirmOpen(true)}
                  disabled={fileDeleting || !selectedConfigWorkspaceID || !filePath.trim()}
                >
                  <Trash2 className="h-4 w-4" />
                  Delete file
                </Button>
              </div>
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
    <Dialog open={fileDeleteConfirmOpen} onOpenChange={setFileDeleteConfirmOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete file?</DialogTitle>
          <DialogDescription>
            {filePath.trim() ? `${filePath.trim()} will be removed from this agent workspace.` : "This file will be removed from this agent workspace."}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setFileDeleteConfirmOpen(false)} disabled={fileDeleting}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={confirmDeleteFile}
            disabled={fileDeleting || !selectedConfigWorkspaceID || !filePath.trim()}
          >
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    </>
  );
}
