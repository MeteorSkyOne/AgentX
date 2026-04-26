import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import {
  Activity,
  ArrowLeft,
  Bot,
  ChevronDown,
  ChevronRight,
  Database,
  FileText,
  Folder,
  FolderClosed,
  FolderOpen,
  Hash,
  Home,
  Key,
  LogOut,
  MessageSquare,
  Moon,
  Pencil,
  Plus,
  Rows3,
  Save,
  Send,
  Settings,
  Sliders,
  SlidersHorizontal,
  Sun,
  Trash2,
  Eye,
  EyeOff,
  UserRound,
  X
} from "lucide-react";
import { cn } from "@/lib/utils";
import { ChannelList } from "./ChannelList";
import { Composer } from "./Composer";
import { MessagePane } from "./MessagePane";
import type {
  Agent,
  Channel,
  ConversationAgentContext,
  ConversationContext,
  ConversationType,
  CreateThreadResponse,
  Message,
  Organization,
  ProcessItem,
  Project,
  Thread,
  User,
  WorkspaceTreeEntry
} from "../api/types";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AgentAvatar,
  AVATAR_COLORS,
  agentKindColor,
  getAgentAvatar,
  setAgentAvatar,
  type AgentAvatarData,
} from "./AgentAvatar";
import type { ThemeMode } from "../theme";

interface ActiveConversation {
  type: ConversationType;
  id: string;
  projectID: string;
  channelID: string;
}

interface StreamingMessage {
  runID: string;
  agentID?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
}

interface ShellProps {
  user: User;
  organization?: Organization;
  projects: Project[];
  project?: Project;
  channels: Channel[];
  selectedChannel?: Channel;
  activeConversation?: ActiveConversation;
  threads: Thread[];
  agents: Agent[];
  channelAgents: ConversationAgentContext[];
  conversationContext?: ConversationContext;
  contextLoading: boolean;
  messages: Message[];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  theme: ThemeMode;
  onSelectProject: (projectID: string) => void;
  onCreateProject: (name: string) => Promise<Project>;
  onSelectChannel: (channel: Channel) => void;
  onCreateChannel: (name: string, type: Channel["type"]) => Promise<Channel>;
  onUpdateChannel: (channelID: string, name: string) => Promise<Channel>;
  onDeleteChannel: (channel: Channel) => Promise<void>;
  onSelectThread: (thread: Thread) => void;
  onCreateThread: (title: string, body: string) => Promise<CreateThreadResponse>;
  onUpdateThread: (threadID: string, title: string) => Promise<Thread>;
  onDeleteThread: (thread: Thread) => Promise<void>;
  onSaveChannelAgents: (
    bindings: Array<{ agent_id: string; run_workspace_id?: string }>
  ) => Promise<void>;
  onCreateAgent: (payload: {
    name: string;
    handle?: string;
    kind?: string;
    model?: string;
    yolo_mode?: boolean;
    env?: Record<string, string>;
  }) => Promise<Agent>;
  onUpdateAgent: (
    agentID: string,
    payload: Partial<Pick<Agent, "name" | "handle" | "kind" | "model" | "enabled" | "yolo_mode">> & {
      env?: Record<string, string>;
    }
  ) => Promise<void>;
  onDeleteAgent: (agentID: string) => Promise<void>;
  onLoadWorkspaceTree: (workspaceID: string) => Promise<WorkspaceTreeEntry>;
  onReadWorkspaceFile: (workspaceID: string, path: string) => Promise<string>;
  onWriteWorkspaceFile: (workspaceID: string, path: string, body: string) => Promise<void>;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onLoadOlderMessages: () => boolean;
  onMessageSent: (message: Message) => void;
  onToggleTheme: () => void;
  onLogout: () => void;
}

export function Shell({
  user,
  organization,
  projects,
  project,
  channels,
  selectedChannel,
  activeConversation,
  threads,
  agents,
  channelAgents,
  conversationContext,
  contextLoading,
  messages,
  messagesLoading,
  olderMessagesLoading,
  hasOlderMessages,
  streaming,
  theme,
  onSelectProject,
  onCreateProject,
  onSelectChannel,
  onCreateChannel,
  onUpdateChannel,
  onDeleteChannel,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread,
  onSaveChannelAgents,
  onCreateAgent,
  onUpdateAgent,
  onDeleteAgent,
  onLoadWorkspaceTree,
  onReadWorkspaceFile,
  onWriteWorkspaceFile,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onMessageSent,
  onToggleTheme,
  onLogout
}: ShellProps) {
  const [agentPanelOpen, setAgentPanelOpen] = useState(false);
  const [membersPanelOpen, setMembersPanelOpen] = useState(false);
  const [projectName, setProjectName] = useState("");
  const [projectDraftOpen, setProjectDraftOpen] = useState(false);
  const [channelDraftOpen, setChannelDraftOpen] = useState(false);
  const [channelName, setChannelName] = useState("");
  const [channelType, setChannelType] = useState<Channel["type"]>("text");
  const [agentDraftOpen, setAgentDraftOpen] = useState(false);
  const [newAgentName, setNewAgentName] = useState("");
  const [newAgentHandle, setNewAgentHandle] = useState("");
  const [newAgentKind, setNewAgentKind] = useState("fake");
  const [newAgentModel, setNewAgentModel] = useState("");
  const [newAgentYoloMode, setNewAgentYoloMode] = useState(false);
  const [newAgentEmoji, setNewAgentEmoji] = useState("");
  const [newAgentColor, setNewAgentColor] = useState("");
  const [newAgentError, setNewAgentError] = useState<string | null>(null);
  const [creatingAgent, setCreatingAgent] = useState(false);
  const [threadEditOpen, setThreadEditOpen] = useState(false);
  const [threadTitleDraft, setThreadTitleDraft] = useState("");
  const [threadActionError, setThreadActionError] = useState<string | null>(null);
  const [threadActionPending, setThreadActionPending] = useState(false);
  const boundAgents = conversationContext?.agents ?? channelAgents;
  const activeAgents = useMemo(() => agents.filter((agent) => agent.enabled), [agents]);
  const selectedAgent = boundAgents[0]?.agent ?? activeAgents[0];
  const activeThread = conversationContext?.thread;

  useEffect(() => {
    setAgentPanelOpen(false);
  }, [selectedChannel?.id, activeConversation?.id]);

  useEffect(() => {
    setThreadTitleDraft(activeThread?.title ?? "");
    setThreadActionError(null);
    setThreadEditOpen(false);
  }, [activeThread?.id]);

  async function submitProject() {
    const name = projectName.trim();
    if (!name) return;
    const created = await onCreateProject(name);
    setProjectName("");
    setProjectDraftOpen(false);
    onSelectProject(created.id);
  }

  async function submitChannel() {
    const name = channelName.trim();
    if (!name) return;
    const created = await onCreateChannel(name, channelType);
    setChannelName("");
    setChannelType("text");
    setChannelDraftOpen(false);
    onSelectChannel(created);
  }

  async function submitAgent() {
    const name = newAgentName.trim();
    if (!name) return;
    const handle = normalizeAgentHandle(newAgentHandle || name);
    if (agents.some((agent) => agent.handle === handle)) {
      setNewAgentError(`Agent @${handle} already exists. Choose a different handle.`);
      return;
    }
    setNewAgentError(null);
    setCreatingAgent(true);
    try {
      const created = await onCreateAgent({
        name,
        handle: newAgentHandle || undefined,
        kind: newAgentKind,
        model: newAgentModel || undefined,
        yolo_mode: newAgentYoloMode,
      });
      if (newAgentEmoji) {
        setAgentAvatar(created.id, { emoji: newAgentEmoji, color: newAgentColor || agentKindColor(newAgentKind) });
      }
      setNewAgentName("");
      setNewAgentHandle("");
      setNewAgentKind("fake");
      setNewAgentModel("");
      setNewAgentYoloMode(false);
      setNewAgentEmoji("");
      setNewAgentColor("");
      setAgentDraftOpen(false);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Create agent failed";
      setNewAgentError(message === "invalid input" ? "Agent name or handle is invalid or already in use." : message);
    } finally {
      setCreatingAgent(false);
    }
  }

  async function submitActiveThreadTitle() {
    if (!activeThread) return;
    const title = threadTitleDraft.trim();
    if (!title) return;
    setThreadActionError(null);
    setThreadActionPending(true);
    try {
      await onUpdateThread(activeThread.id, title);
      setThreadEditOpen(false);
    } catch (err) {
      setThreadActionError(err instanceof Error ? err.message : "Update post failed");
    } finally {
      setThreadActionPending(false);
    }
  }

  async function deleteActiveThread() {
    if (!activeThread) return;
    if (!window.confirm(`Delete "${activeThread.title}"?`)) return;
    setThreadActionError(null);
    setThreadActionPending(true);
    try {
      await onDeleteThread(activeThread);
    } catch (err) {
      setThreadActionError(err instanceof Error ? err.message : "Delete post failed");
    } finally {
      setThreadActionPending(false);
    }
  }

  const title = conversationTitle(selectedChannel, activeThread, boundAgents.map((item) => item.agent));
  const subtitle = conversationSubtitle(selectedChannel, activeThread, boundAgents.length);
  const composerConversation =
    activeConversation && selectedChannel?.type === "text"
      ? { type: activeConversation.type, id: activeConversation.id, label: `#${selectedChannel.name}` }
      : activeConversation && activeThread
        ? { type: activeConversation.type, id: activeConversation.id, label: activeThread.title }
        : undefined;

  return (
    <div className="flex h-screen w-screen">
      {/* Project Rail */}
      <TooltipProvider delayDuration={0}>
        <div className="flex h-full w-[72px] flex-col items-center gap-2 bg-sidebar py-3">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground font-bold text-lg"
              >
                AX
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">AgentX</TooltipContent>
          </Tooltip>

          <div className="mx-auto h-0.5 w-8 rounded-full bg-border" />

          <ScrollArea className="min-h-0 w-full flex-1">
            <div className="flex flex-col items-center gap-2">
              {projects.map((item) => (
                <Tooltip key={item.id}>
                  <TooltipTrigger asChild>
                    <button
                      className={cn(
                        "relative flex h-12 w-12 items-center justify-center rounded-2xl bg-secondary text-secondary-foreground transition-all hover:rounded-xl hover:bg-primary hover:text-primary-foreground",
                        item.id === project?.id &&
                          "rounded-xl bg-primary text-primary-foreground"
                      )}
                      title={item.name}
                      aria-label={item.name}
                      onClick={() => onSelectProject(item.id)}
                    >
                      <span className="text-lg font-semibold">{initials(item.name)}</span>
                      {item.id === project?.id && (
                        <div className="absolute -left-3 h-10 w-1 rounded-r-full bg-foreground" />
                      )}
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="right">{item.name}</TooltipContent>
                </Tooltip>
              ))}
            </div>
          </ScrollArea>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className={cn(
                  "flex h-12 w-12 items-center justify-center rounded-2xl bg-secondary text-muted-foreground transition-all hover:rounded-xl hover:bg-green-600 hover:text-white",
                  projectDraftOpen && "rounded-xl bg-green-600 text-white"
                )}
                title="Create project"
                aria-label="Create project"
                onClick={() => setProjectDraftOpen(true)}
              >
                <Plus className="h-5 w-5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">Create project</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-10 w-10 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
                title={theme === "dark" ? "Light mode" : "Dark mode"}
                aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
                onClick={onToggleTheme}
              >
                {theme === "dark" ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">
              {theme === "dark" ? "Light mode" : "Dark mode"}
            </TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-10 w-10 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
                title="Log out"
                aria-label="Log out"
                onClick={onLogout}
              >
                <LogOut className="h-5 w-5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">Log out</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>

      {/* Main Content */}
      <ResizablePanelGroup direction="horizontal" className="flex-1">
        {/* Channel Sidebar */}
        <ResizablePanel defaultSize={18} minSize={15} maxSize={25}>
          <div className="flex h-full flex-col bg-card">
            {/* Workspace Header */}
            <div className="flex h-12 items-center justify-between border-b border-border px-4">
              <h2 className="truncate text-base font-semibold">
                {project?.name ?? "No project"}
              </h2>
              <Button variant="ghost" size="icon" className="h-8 w-8">
                <ChevronDown className="h-4 w-4" />
              </Button>
            </div>

            <ScrollArea className="min-h-0 flex-1">
              <div className="px-2 py-3">
                {/* Channels */}
                <ChannelList
                  channels={channels}
                  selectedChannelID={selectedChannel?.id}
                  onSelect={onSelectChannel}
                  onCreate={() => setChannelDraftOpen(true)}
                  onUpdate={onUpdateChannel}
                  onDelete={onDeleteChannel}
                />

                {/* Agents Section */}
                <AgentsSidebar
                  agents={activeAgents}
                  boundAgents={boundAgents}
                  contextLoading={contextLoading}
                  onOpenPanel={() => setAgentPanelOpen(true)}
                  onCreateAgent={() => setAgentDraftOpen(true)}
                />
              </div>
            </ScrollArea>

            {/* User Info */}
            <div className="flex items-center gap-2 border-t border-border bg-sidebar p-2">
              <Avatar className="h-8 w-8">
                <AvatarFallback className="bg-primary text-primary-foreground text-xs">
                  {initials(user.display_name)}
                </AvatarFallback>
              </Avatar>
              <div className="flex-1 truncate">
                <p className="text-sm font-medium">{user.display_name}</p>
                <p className="text-xs text-muted-foreground">online</p>
              </div>
              <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onLogout}>
                <Settings className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </ResizablePanel>

        <ResizableHandle withHandle />

        {/* Message Area */}
        <ResizablePanel defaultSize={(agentPanelOpen || membersPanelOpen) ? 55 : 82}>
          <div className="flex h-full flex-1 flex-col bg-background">
            {/* Channel Header */}
            <div className="flex h-12 items-center justify-between border-b border-border px-4">
              <div className="flex items-center gap-2">
                {selectedChannel?.type === "thread" && activeThread ? (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    title="Back to posts"
                    aria-label="Back to posts"
                    onClick={() => onSelectChannel(selectedChannel)}
                  >
                    <ArrowLeft className="h-4 w-4" />
                  </Button>
                ) : null}
                {selectedChannel?.type === "thread" && !activeThread ? (
                  <Rows3 className="h-5 w-5 text-muted-foreground" />
                ) : boundAgents.length === 1 ? (
                  <Bot className={cn("h-5 w-5", agentToneColor(boundAgents[0].agent.kind))} />
                ) : (
                  <Hash className="h-5 w-5 text-muted-foreground" />
                )}
                <div>
                  <h1 className="text-sm font-semibold">{title}</h1>
                  <p className="text-xs text-muted-foreground">{subtitle}</p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {activeThread && (
                  <>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      title="Edit post"
                      aria-label="Edit post"
                      onClick={() => {
                        setThreadTitleDraft(activeThread.title);
                        setThreadActionError(null);
                        setThreadEditOpen(true);
                      }}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-muted-foreground hover:text-destructive"
                      title="Delete post"
                      aria-label="Delete post"
                      disabled={threadActionPending}
                      onClick={deleteActiveThread}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </>
                )}
                {activeConversation && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Activity className="h-3.5 w-3.5" />
                    {streaming.length > 0 ? "running" : "ready"}
                  </span>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-8 w-8", membersPanelOpen && "bg-accent")}
                  title="Members"
                  aria-label="Members"
                  onClick={() => setMembersPanelOpen((open) => !open)}
                >
                  <UserRound className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  title="Agent settings"
                  aria-label="Agent settings"
                  onClick={() => setAgentPanelOpen((open) => !open)}
                >
                  <SlidersHorizontal className="h-4 w-4" />
                </Button>
              </div>
            </div>

            {selectedChannel?.type === "thread" && !activeThread ? (
              <ThreadForum
                threads={threads}
                onSelectThread={onSelectThread}
                onCreateThread={onCreateThread}
                onUpdateThread={onUpdateThread}
                onDeleteThread={onDeleteThread}
              />
            ) : (
              <>
                <MessagePane
                  messages={messages}
                  isLoading={messagesLoading}
                  isLoadingOlder={olderMessagesLoading}
                  hasOlderMessages={hasOlderMessages}
                  streaming={streaming}
                  agents={boundAgents}
                  onUpdateMessage={onUpdateMessage}
                  onDeleteMessage={onDeleteMessage}
                  onLoadOlder={onLoadOlderMessages}
                />
                <Composer
                  conversation={composerConversation}
                  mentionAgents={boundAgents.map((item) => item.agent)}
                  typingAgents={streaming
                    .filter((s) => !s.error)
                    .map((s) => {
                      const agent = boundAgents.find((b) => b.agent.id === s.agentID);
                      return { name: agent?.agent.name ?? "Agent" };
                    })}
                  onSent={onMessageSent}
                />
              </>
            )}
          </div>
        </ResizablePanel>

        {/* Members Panel */}
        {membersPanelOpen && !agentPanelOpen && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel defaultSize={20} minSize={15} maxSize={30}>
              <MembersPanel
                agents={activeAgents}
                boundAgents={boundAgents}
                selectedChannel={selectedChannel}
                onSaveChannelAgents={onSaveChannelAgents}
                onClose={() => setMembersPanelOpen(false)}
              />
            </ResizablePanel>
          </>
        )}

        {/* Agent Panel */}
        {agentPanelOpen && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel defaultSize={27} minSize={20} maxSize={35}>
              <AgentDetailsPanel
                selectedChannel={selectedChannel}
                agents={activeAgents}
                boundAgents={boundAgents}
                selectedAgent={selectedAgent}
                onSaveChannelAgents={onSaveChannelAgents}
                onUpdateAgent={onUpdateAgent}
                onDeleteAgent={onDeleteAgent}
                onLoadWorkspaceTree={onLoadWorkspaceTree}
                onReadWorkspaceFile={onReadWorkspaceFile}
                onWriteWorkspaceFile={onWriteWorkspaceFile}
                onCreateAgentModal={() => setAgentDraftOpen(true)}
                onClose={() => setAgentPanelOpen(false)}
              />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>

      {/* Edit Post Modal */}
      <Dialog open={threadEditOpen} onOpenChange={setThreadEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit post</DialogTitle>
            <DialogDescription>Update the post title shown in the forum catalog.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-2">
            <Label htmlFor="thread-title">Title</Label>
            <Input
              id="thread-title"
              value={threadTitleDraft}
              onChange={(e) => setThreadTitleDraft(e.target.value)}
              aria-label="Post title"
              onKeyDown={(e) => { if (e.key === "Enter") submitActiveThreadTitle(); }}
              autoFocus
            />
            {threadActionError && <p className="text-sm text-destructive">{threadActionError}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setThreadEditOpen(false)} disabled={threadActionPending}>Cancel</Button>
            <Button onClick={submitActiveThreadTitle} disabled={!threadTitleDraft.trim() || threadActionPending}>Save</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Project Modal */}
      <Dialog open={projectDraftOpen} onOpenChange={setProjectDraftOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create project</DialogTitle>
            <DialogDescription>Add a new project to organize your channels and agents.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="project-name">Project name</Label>
              <Input
                id="project-name"
                value={projectName}
                onChange={(e) => setProjectName(e.target.value)}
                placeholder="My Project"
                aria-label="Project name"
                onKeyDown={(e) => { if (e.key === "Enter") submitProject(); }}
                autoFocus
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setProjectDraftOpen(false)}>Cancel</Button>
            <Button onClick={submitProject} disabled={!projectName.trim()}>Save</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Channel Modal */}
      <Dialog open={channelDraftOpen} onOpenChange={setChannelDraftOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create channel</DialogTitle>
            <DialogDescription>Add a new channel to this project.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="channel-name">Channel name</Label>
              <Input
                id="channel-name"
                value={channelName}
                onChange={(e) => setChannelName(e.target.value)}
                placeholder="general"
                aria-label="Channel name"
                onKeyDown={(e) => { if (e.key === "Enter") submitChannel(); }}
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="channel-type">Channel type</Label>
              <select
                id="channel-type"
                value={channelType}
                onChange={(e) => setChannelType(e.target.value as Channel["type"])}
                aria-label="Channel type"
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="text">Text</option>
                <option value="thread">Forum</option>
              </select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChannelDraftOpen(false)}>Cancel</Button>
            <Button onClick={submitChannel} disabled={!channelName.trim()}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Agent Modal */}
      <Dialog
        open={agentDraftOpen}
        onOpenChange={(open) => {
          setAgentDraftOpen(open);
          if (open) setNewAgentError(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create agent</DialogTitle>
            <DialogDescription>Add a new agent to this organization.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {/* Avatar */}
            <div className="flex items-center gap-4">
              <div className={cn(
                "flex h-14 w-14 items-center justify-center rounded-full shrink-0",
                newAgentEmoji
                  ? (newAgentColor || agentKindColor(newAgentKind))
                  : agentKindColor(newAgentKind)
              )}>
                {newAgentEmoji ? (
                  <span className="text-2xl">{newAgentEmoji}</span>
                ) : (
                  <Bot className="h-7 w-7 text-white" />
                )}
              </div>
              <div className="flex-1 space-y-2">
                <Input
                  value={newAgentEmoji}
                  onChange={(e) => setNewAgentEmoji(e.target.value)}
                  placeholder="Avatar emoji (e.g. 🤖)"
                  aria-label="New agent avatar"
                />
                <div className="flex gap-1.5 flex-wrap">
                  {AVATAR_COLORS.map((c) => (
                    <button
                      key={c}
                      className={cn(
                        "h-5 w-5 rounded-full transition-all",
                        c,
                        newAgentColor === c ? "ring-2 ring-ring ring-offset-1 ring-offset-background" : "opacity-60 hover:opacity-100"
                      )}
                      onClick={() => setNewAgentColor(c)}
                      type="button"
                    />
                  ))}
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="new-agent-name">Name</Label>
              <Input
                id="new-agent-name"
                value={newAgentName}
                onChange={(e) => {
                  setNewAgentName(e.target.value);
                  setNewAgentError(null);
                }}
                placeholder="My Agent"
                aria-label="New agent name"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="new-agent-handle">Handle</Label>
              <Input
                id="new-agent-handle"
                value={newAgentHandle}
                onChange={(e) => {
                  setNewAgentHandle(e.target.value);
                  setNewAgentError(null);
                }}
                placeholder="my_agent"
                aria-label="New agent handle"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="new-agent-runtime">Runtime</Label>
                <select
                  id="new-agent-runtime"
                  value={newAgentKind}
                  onChange={(e) => setNewAgentKind(e.target.value)}
                  aria-label="New agent runtime"
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="fake">Fake</option>
                  <option value="codex">Codex</option>
                  <option value="claude">Claude</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-agent-model">Model</Label>
                <Input
                  id="new-agent-model"
                  value={newAgentModel}
                  onChange={(e) => setNewAgentModel(e.target.value)}
                  placeholder="default"
                  aria-label="New agent model"
                />
              </div>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={newAgentYoloMode}
                onChange={(e) => setNewAgentYoloMode(e.target.checked)}
                className="rounded border-border"
                aria-label="New agent YOLO mode"
              />
              YOLO mode
            </label>
            {newAgentError && (
              <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {newAgentError}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAgentDraftOpen(false)} disabled={creatingAgent}>Cancel</Button>
            <Button onClick={submitAgent} disabled={!newAgentName.trim() || creatingAgent}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function AgentsSidebar({
  agents,
  boundAgents,
  contextLoading,
  onOpenPanel,
  onCreateAgent,
}: {
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  contextLoading: boolean;
  onOpenPanel: () => void;
  onCreateAgent: () => void;
}) {
  const [open, setOpen] = useState(true);

  return (
    <section aria-label="Bound agent">
      <Collapsible open={open} onOpenChange={setOpen} className="mt-4">
        <CollapsibleTrigger asChild>
          <button className="flex w-full items-center gap-1 px-1 py-1 text-xs font-semibold uppercase text-muted-foreground hover:text-foreground">
            {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            Agents
            {contextLoading && <span className="ml-1 h-1.5 w-1.5 rounded-full bg-yellow-500 animate-pulse" />}
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-0.5">
          {boundAgents.map((item) => (
            <button
              key={item.agent.id}
              className="group flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-foreground"
              aria-label={item.agent.name}
              onClick={onOpenPanel}
            >
              <div className="relative">
                <AgentAvatar agentID={item.agent.id} kind={item.agent.kind} size="sm" className="h-5 w-5" />
                <div className={cn(
                  "absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border border-card",
                  item.agent.enabled ? "bg-green-500" : "bg-gray-500"
                )} />
              </div>
              <span className="truncate">{item.agent.name}</span>
              <Settings className="ml-auto h-3 w-3 opacity-0 group-hover:opacity-100" />
            </button>
          ))}
          {boundAgents.length === 0 && (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">Unbound</p>
          )}
          <button
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            onClick={onCreateAgent}
          >
            <Plus className="h-4 w-4" />
            <span>Create agent</span>
          </button>
          <button
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            onClick={onOpenPanel}
          >
            <Settings className="h-4 w-4" />
            <span>Manage agents</span>
          </button>
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}

function MembersPanel({
  agents,
  boundAgents,
  selectedChannel,
  onSaveChannelAgents,
  onClose,
}: {
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  selectedChannel?: Channel;
  onSaveChannelAgents: ShellProps["onSaveChannelAgents"];
  onClose: () => void;
}) {
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const next: Record<string, boolean> = {};
    for (const item of boundAgents) {
      next[item.agent.id] = true;
    }
    setChecked(next);
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
        .map((a) => {
          const existing = boundAgents.find((b) => b.agent.id === a.id);
          return {
            agent_id: a.id,
            run_workspace_id: existing?.binding.run_workspace_id || undefined,
          };
        });
      await onSaveChannelAgents(bindings);
      setDirty(false);
    } finally {
      setSaving(false);
    }
  }

  const bound = agents.filter((a) => checked[a.id]);
  const unbound = agents.filter((a) => !checked[a.id]);

  return (
    <aside className="flex h-full flex-col border-l border-border bg-card" aria-label="Channel members">
      <div className="flex h-12 items-center justify-between border-b border-border px-4">
        <span className="text-sm font-semibold">Members</span>
        <Button variant="ghost" size="icon" className="h-8 w-8" title="Close" aria-label="Close members" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <ScrollArea className="flex-1">
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
                <label
                  key={a.id}
                  className="picker-row flex items-center gap-2.5 rounded-md px-2 py-2 hover:bg-accent/50 cursor-pointer"
                >
                  <input
                    type="checkbox"
                    checked
                    onChange={() => toggle(a.id, false)}
                    className="rounded border-border"
                  />
                  <div className="relative">
                    <AgentAvatar agentID={a.id} kind={a.kind} size="sm" />
                    <div className={cn(
                      "absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-card",
                      a.enabled ? "bg-green-500" : "bg-gray-500"
                    )} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{a.name}</p>
                    <p className="text-xs text-muted-foreground truncate">@{a.handle}</p>
                  </div>
                  <Badge variant="outline" className={cn("text-[10px] shrink-0", agentToneColor(a.kind))}>
                    {agentKindLabel(a.kind)}
                  </Badge>
                </label>
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
                  <input
                    type="checkbox"
                    checked={false}
                    onChange={() => toggle(a.id, true)}
                    className="rounded border-border"
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
        <div className="border-t border-border p-3">
          <Button size="sm" className="w-full gap-2" onClick={save} disabled={saving}>
            <Save className="h-4 w-4" />
            Save
          </Button>
        </div>
      )}
    </aside>
  );
}

function ThreadForum({
  threads,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread
}: {
  threads: Thread[];
  onSelectThread: (thread: Thread) => void;
  onCreateThread: (title: string, body: string) => Promise<CreateThreadResponse>;
  onUpdateThread: (threadID: string, title: string) => Promise<Thread>;
  onDeleteThread: (thread: Thread) => Promise<void>;
}) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [draftTitle, setDraftTitle] = useState("");
  const [pendingID, setPendingID] = useState<string | null>(null);

  async function submit() {
    if (!title.trim() || !body.trim()) return;
    setError(null);
    try {
      const created = await onCreateThread(title, body);
      setTitle("");
      setBody("");
      onSelectThread(created.thread);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Post failed");
    }
  }

  function beginEdit(thread: Thread) {
    setEditingID(thread.id);
    setDraftTitle(thread.title);
    setError(null);
  }

  async function saveThread(thread: Thread) {
    const nextTitle = draftTitle.trim();
    if (!nextTitle) return;
    setPendingID(thread.id);
    setError(null);
    try {
      await onUpdateThread(thread.id, nextTitle);
      setEditingID(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update post failed");
    } finally {
      setPendingID(null);
    }
  }

  async function removeThread(thread: Thread) {
    if (!window.confirm(`Delete "${thread.title}"?`)) return;
    setPendingID(thread.id);
    setError(null);
    try {
      await onDeleteThread(thread);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete post failed");
    } finally {
      setPendingID(null);
    }
  }

  return (
    <section className="flex flex-1 flex-col overflow-hidden p-4" aria-label="Threads">
      <ScrollArea className="flex-1">
        <div className="space-y-1">
          {threads.map((thread) => (
            <div
              key={thread.id}
              className="group flex w-full items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-accent/50"
            >
              {editingID === thread.id ? (
                <>
                  <Input
                    value={draftTitle}
                    onChange={(e) => setDraftTitle(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void saveThread(thread);
                      if (e.key === "Escape") setEditingID(null);
                    }}
                    aria-label="Post title"
                    className="h-8 flex-1"
                    autoFocus
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    title="Save post"
                    aria-label="Save post"
                    disabled={pendingID === thread.id || !draftTitle.trim()}
                    onClick={() => saveThread(thread)}
                  >
                    <Save className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    title="Cancel"
                    aria-label="Cancel"
                    disabled={pendingID === thread.id}
                    onClick={() => setEditingID(null)}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </>
              ) : (
                <>
                  <button
                    className="flex min-w-0 flex-1 items-center gap-3 rounded px-1 py-1 text-left"
                    onClick={() => onSelectThread(thread)}
                  >
                    <MessageSquare className="h-4 w-4 text-primary shrink-0" />
                    <span className="flex-1 truncate font-medium">{thread.title}</span>
                    <time className="shrink-0 text-xs text-muted-foreground">{formatDate(thread.updated_at)}</time>
                  </button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100"
                    title="Edit post"
                    aria-label="Edit post"
                    disabled={pendingID === thread.id}
                    onClick={() => beginEdit(thread)}
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100 focus:opacity-100"
                    title="Delete post"
                    aria-label="Delete post"
                    disabled={pendingID === thread.id}
                    onClick={() => removeThread(thread)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </>
              )}
            </div>
          ))}
          {threads.length === 0 && (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <MessageSquare className="h-10 w-10 mb-2" />
              <span className="text-sm">No posts yet</span>
            </div>
          )}
        </div>
      </ScrollArea>

      <div className="mt-4 space-y-2 rounded-lg border border-border p-3">
        <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Title" aria-label="Post title" />
        <Textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder="Body" aria-label="Post body" rows={3} />
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button className="w-full" onClick={submit}>Create post</Button>
      </div>
    </section>
  );
}

function AgentDetailsPanel({
  selectedChannel,
  agents,
  boundAgents,
  selectedAgent,
  onSaveChannelAgents,
  onUpdateAgent,
  onDeleteAgent,
  onLoadWorkspaceTree,
  onReadWorkspaceFile,
  onWriteWorkspaceFile,
  onCreateAgentModal,
  onClose
}: {
  selectedChannel?: Channel;
  agents: Agent[];
  boundAgents: ConversationContext["agents"];
  selectedAgent?: Agent;
  onSaveChannelAgents: (
    bindings: Array<{ agent_id: string; run_workspace_id?: string }>
  ) => Promise<void>;
  onUpdateAgent: ShellProps["onUpdateAgent"];
  onDeleteAgent: ShellProps["onDeleteAgent"];
  onLoadWorkspaceTree: ShellProps["onLoadWorkspaceTree"];
  onReadWorkspaceFile: ShellProps["onReadWorkspaceFile"];
  onWriteWorkspaceFile: ShellProps["onWriteWorkspaceFile"];
  onCreateAgentModal: () => void;
  onClose: () => void;
}) {
  const [checkedAgents, setCheckedAgents] = useState<Record<string, boolean>>({});
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [agentID, setAgentID] = useState(selectedAgent?.id ?? "");
  const [name, setName] = useState(selectedAgent?.name ?? "");
  const [handle, setHandle] = useState(selectedAgent?.handle ?? "");
  const [kind, setKind] = useState(selectedAgent?.kind ?? "fake");
  const [model, setModel] = useState(selectedAgent?.model ?? "");
  const [enabled, setEnabled] = useState(selectedAgent?.enabled ?? true);
  const [yoloMode, setYoloMode] = useState(selectedAgent?.yolo_mode ?? false);
  const [avatarEmoji, setAvatarEmoji] = useState("");
  const [avatarColor, setAvatarColor] = useState("");
  const [filePath, setFilePath] = useState("memory.md");
  const [fileBody, setFileBody] = useState("");
  const [tree, setTree] = useState<WorkspaceTreeEntry>();
  const [envBody, setEnvBody] = useState("{}");
  const [status, setStatus] = useState<string | null>(null);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const selected = agents.find((a) => a.id === agentID) ?? selectedAgent;
  const selectedBinding = boundAgents.find((item) => item.agent.id === selected?.id);
  const envEntries = useMemo(
    () => Object.entries(selected?.env ?? {}).sort(([l], [r]) => l.localeCompare(r)),
    [selected?.env]
  );

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
    setHandle(selected.handle);
    setKind(selected.kind);
    setModel(selected.model);
    setEnabled(selected.enabled);
    setYoloMode(selected.yolo_mode);
    setEnvBody("{}");
    const av = getAgentAvatar(selected.id);
    setAvatarEmoji(av?.emoji ?? "");
    setAvatarColor(av?.color ?? "");
    setDeleteConfirmOpen(false);
  }, [selected?.id]);

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
    await onUpdateAgent(selected.id, { name, handle, kind, model, enabled, yolo_mode: yoloMode });
    setAgentAvatar(selected.id, avatarEmoji ? { emoji: avatarEmoji, color: avatarColor || agentKindColor(kind) } : null);
    setStatus("Saved");
  }

  async function loadFile() {
    if (!selected) return;
    const body = await onReadWorkspaceFile(selected.config_workspace_id, filePath);
    setFileBody(body);
    setStatus("Loaded");
  }

  async function loadTree() {
    if (!selected) return;
    const nextTree = await onLoadWorkspaceTree(selected.config_workspace_id);
    setTree(nextTree);
    setStatus("Loaded");
  }

  async function saveFile() {
    if (!selected) return;
    await onWriteWorkspaceFile(selected.config_workspace_id, filePath, fileBody);
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
    <aside className="flex h-full flex-col border-l border-border bg-card" aria-label="Agent details">
      {/* Header */}
      <div className="flex h-12 items-center justify-between border-b border-border px-4">
        <div className="flex items-center gap-2">
          <Bot className={cn("h-5 w-5", agentColor)} />
          <span className="font-semibold">Agent Config</span>
        </div>
        <Button variant="ghost" size="icon" className="h-8 w-8" title="Close" aria-label="Close" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      {/* Agent Info */}
      <div className="border-b border-border p-4">
        <div className="flex items-center gap-3">
          {selected ? (
            <AgentAvatar agentID={selected.id} kind={selected.kind} size="lg" className="rounded-xl" />
          ) : (
            <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary">
              <Bot className="h-6 w-6 text-white" />
            </div>
          )}
          <div className="flex-1">
            <h2 className="font-semibold">{selected?.name ?? "Agents"}</h2>
            <div className="flex items-center gap-2">
              <Badge variant="outline" className={cn("text-xs", agentColor)}>
                {agentKindLabel(selected?.kind ?? "fake")}
              </Badge>
              <span className="text-xs text-muted-foreground">
                {selected?.handle ?? ""}
              </span>
            </div>
          </div>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <span className="text-muted-foreground">Model</span>
          <strong>{selected?.model || "default"}</strong>
          <span className="text-muted-foreground">YOLO</span>
          <strong>{selected?.yolo_mode ? "on" : "off"}</strong>
          <span className="text-muted-foreground">Channel</span>
          <strong>{selectedChannel ? `#${selectedChannel.name}` : "none"}</strong>
        </div>
        <div className="workspace-path mt-2 flex items-center gap-1 text-xs text-muted-foreground">
          <Database className="h-3 w-3" />
          <span>{selectedBinding?.config_workspace.path ?? selected?.config_workspace_id ?? ""}</span>
        </div>
        <div className="mt-1 text-xs text-muted-foreground">
          {envEntries.length > 0
            ? envEntries.map(([k]) => k).join(", ")
            : "empty"}
        </div>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="settings" className="flex flex-1 flex-col overflow-hidden">
        <TabsList className="mx-4 mt-4 grid w-auto grid-cols-4">
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
        <TabsContent value="settings" className="flex-1 overflow-hidden px-4 pb-4">
          <ScrollArea className="h-full">
            <div className="space-y-4 pr-2">
              {/* Agent Selector */}
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground uppercase">Agent</Label>
                <select
                  value={agentID}
                  onChange={(e) => setAgentID(e.target.value)}
                  aria-label="Agent"
                  className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
                >
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>{a.name} (@{a.handle})</option>
                  ))}
                </select>
              </div>

              {/* Edit Fields */}
              <div className="space-y-2">
                <Label className="text-xs">Name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} aria-label="Agent name" />
              </div>
              <div className="space-y-2">
                <Label className="text-xs">Handle</Label>
                <Input value={handle} onChange={(e) => setHandle(e.target.value)} aria-label="Agent handle" />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-2">
                  <Label className="text-xs">Runtime</Label>
                  <select
                    value={kind}
                    onChange={(e) => setKind(e.target.value)}
                    aria-label="Agent runtime"
                    className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
                  >
                    <option value="fake">Fake</option>
                    <option value="codex">Codex</option>
                    <option value="claude">Claude</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label className="text-xs">Model</Label>
                  <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="Model" aria-label="Agent model" />
                </div>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={enabled}
                  onChange={(e) => setEnabled(e.target.checked)}
                  className="rounded border-border"
                />
                Enabled
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={yoloMode}
                  onChange={(e) => setYoloMode(e.target.checked)}
                  className="rounded border-border"
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
        <TabsContent value="members" className="flex-1 overflow-hidden px-4 pb-4">
          <ScrollArea className="h-full">
            <div className="space-y-3 pr-2">
              <p className="text-xs text-muted-foreground">
                {selectedChannel ? `#${selectedChannel.name}` : "No channel selected"}
              </p>
              {agents.map((a) => (
                <label key={a.id} className="picker-row flex items-center gap-2 rounded-lg border border-border p-3">
                  <input
                    type="checkbox"
                    checked={Boolean(checkedAgents[a.id])}
                    onChange={(e) =>
                      setCheckedAgents((c) => ({ ...c, [a.id]: e.target.checked }))
                    }
                    className="rounded border-border"
                  />
                  <div className="flex-1">
                    <span className="text-sm font-medium">{a.name}</span>
                    <Input
                      value={overrides[a.id] ?? ""}
                      onChange={(e) => setOverrides((c) => ({ ...c, [a.id]: e.target.value }))}
                      placeholder="Workspace override"
                      aria-label={`${a.name} workspace override`}
                      className="mt-1 text-xs"
                    />
                  </div>
                </label>
              ))}
              <Button size="sm" className="w-full gap-2" onClick={saveBindings}>
                <Save className="h-4 w-4" />
                Save
              </Button>
            </div>
          </ScrollArea>
        </TabsContent>

        {/* Workspace Tab */}
        <TabsContent value="workspace" className="flex flex-1 flex-col overflow-hidden px-4 pb-4">
          <div className="flex flex-1 flex-col gap-3 overflow-hidden">
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Database className="h-3.5 w-3.5" />
              <span className="truncate">{selectedBinding?.config_workspace.path ?? selected?.config_workspace_id ?? ""}</span>
            </div>
            <div className="flex gap-2">
              <Input value={filePath} onChange={(e) => setFilePath(e.target.value)} aria-label="File path" className="flex-1 text-xs" />
              <Button size="sm" variant="outline" onClick={loadFile}>Open</Button>
              <Button size="sm" variant="outline" onClick={loadTree}>Tree</Button>
            </div>
            {tree && <WorkspaceTreeView tree={tree} onSelectPath={setFilePath} />}
            <Textarea
              className="flex-1 resize-none font-mono text-xs"
              value={fileBody}
              onChange={(e) => setFileBody(e.target.value)}
              placeholder="File content..."
              aria-label="File content"
            />
            <Button size="sm" className="w-full gap-2" onClick={saveFile}>
              <FileText className="h-4 w-4" />
              Save file
            </Button>
          </div>
        </TabsContent>

        {/* Env Tab */}
        <TabsContent value="env" className="flex-1 overflow-hidden px-4 pb-4">
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

function WorkspaceTreeView({
  tree,
  onSelectPath
}: {
  tree: WorkspaceTreeEntry;
  onSelectPath: (path: string) => void;
}) {
  const entries = flattenTree(tree).filter((entry) => entry.path !== "");
  if (entries.length === 0) {
    return <p className="text-xs text-muted-foreground">empty</p>;
  }
  return (
    <ScrollArea className="max-h-40 rounded-md border border-border bg-background/50">
      <div className="p-2">
        {entries.map((entry) => (
          <button
            key={entry.path}
            className={cn(
              "flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-xs transition-colors",
              entry.type === "directory"
                ? "text-muted-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            )}
            style={{ paddingLeft: `${entry.depth * 12 + 6}px` }}
            disabled={entry.type === "directory"}
            onClick={() => onSelectPath(entry.path)}
          >
            {entry.type === "directory" ? (
              <Folder className="h-3.5 w-3.5 shrink-0 text-yellow-500" />
            ) : (
              <FileText className="h-3.5 w-3.5 shrink-0 text-blue-400" />
            )}
            <span className="truncate">{entry.name}</span>
          </button>
        ))}
      </div>
    </ScrollArea>
  );
}

function flattenTree(tree: WorkspaceTreeEntry, depth = 0): Array<WorkspaceTreeEntry & { depth: number }> {
  const current = { ...tree, depth };
  const children = tree.children?.flatMap((child) => flattenTree(child, depth + 1)) ?? [];
  return [current, ...children];
}

function conversationTitle(
  channel: Channel | undefined,
  thread: Thread | undefined,
  agents: Agent[]
): string {
  if (thread) return thread.title;
  if (channel?.type === "thread") return channel.name;
  if (agents.length === 1) return agents[0].name;
  return channel?.name ?? "No channel";
}

function conversationSubtitle(
  channel: Channel | undefined,
  thread: Thread | undefined,
  agentCount: number
): string {
  if (!channel) return "No channel selected";
  if (thread) return `#${channel.name} · ${agentCount} agents`;
  return `${channel.type === "thread" ? "forum" : "text"} · ${agentCount} agents`;
}

function agentToneColor(kind: string): string {
  if (kind === "codex") return "text-[oklch(0.7_0.2_145)]";
  if (kind === "claude") return "text-[oklch(0.75_0.15_50)]";
  return "text-muted-foreground";
}

function agentKindLabel(kind: string): string {
  switch (kind) {
    case "codex": return "Codex";
    case "claude": return "Claude Code";
    case "fake": return "Fake runtime";
    default: return kind || "Agent";
  }
}

function initials(value: string): string {
  return (
    value
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() ?? "")
      .join("") || "AX"
  );
}

function normalizeAgentHandle(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[\s.]+/g, "_")
    .replace(/[^a-z0-9_-]+/g, "")
    .replace(/[_-]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit"
  }).format(new Date(value));
}
