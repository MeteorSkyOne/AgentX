import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  Activity,
  ArrowLeft,
  Bot,
  ChevronDown,
  ChevronRight,
  Database,
  FileText,
  FolderOpen,
  Hash,
  Home,
  Key,
  LogOut,
  Menu,
  MessageSquare,
  Moon,
  Pencil,
  Plus,
  RefreshCw,
  Rows3,
  Save,
  Send,
  Settings,
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
import { FileTree } from "./FileTree";
import { MessagePane } from "./MessagePane";
import type {
  Agent,
  Channel,
  ConversationAgentContext,
  ConversationContext,
  ConversationType,
  CreateThreadResponse,
  Message,
  NotificationSettings,
  Organization,
  ProcessItem,
  Project,
  Thread,
  User,
  Workspace,
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
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
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
} from "./AgentAvatar";
import type { ThemeMode } from "../theme";
import {
  browserNotificationPermission,
  requestBrowserNotificationPermission,
  type BrowserNotificationPermission,
} from "../notifications/browser";

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

interface ComposerConversation {
  type: ConversationType;
  id: string;
  label: string;
}

interface ShellProps {
  user: User;
  organization?: Organization;
  projects: Project[];
  project?: Project;
  projectWorkspace?: Workspace;
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
  notificationSettings?: NotificationSettings;
  notificationSettingsLoading: boolean;
  theme: ThemeMode;
  onSelectProject: (projectID: string) => void;
  onCreateProject: (name: string) => Promise<Project>;
  onUpdateProject: (
    projectID: string,
    payload: { name?: string; workspace_path?: string }
  ) => Promise<Project>;
  onDeleteProject: (project: Project) => Promise<void>;
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
    description?: string;
    handle?: string;
    kind?: string;
    model?: string;
    effort?: string;
    fast_mode?: boolean;
    yolo_mode?: boolean;
    env?: Record<string, string>;
  }) => Promise<Agent>;
  onUpdateAgent: (
    agentID: string,
    payload: Partial<Pick<Agent, "name" | "description" | "handle" | "kind" | "model" | "effort" | "enabled" | "fast_mode" | "yolo_mode">> & {
      env?: Record<string, string>;
    }
  ) => Promise<void>;
  onDeleteAgent: (agentID: string) => Promise<void>;
  onUpdateNotificationSettings: (payload: {
    webhook_enabled: boolean;
    webhook_url: string;
    webhook_secret?: string;
  }) => Promise<NotificationSettings>;
  onTestNotificationSettings: () => Promise<void>;
  onLoadWorkspaceTree: (workspaceID: string) => Promise<WorkspaceTreeEntry>;
  onReadWorkspaceFile: (workspaceID: string, path: string) => Promise<string>;
  onWriteWorkspaceFile: (workspaceID: string, path: string, body: string) => Promise<void>;
  onDeleteWorkspaceFile: (workspaceID: string, path: string) => Promise<void>;
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
  projectWorkspace,
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
  notificationSettings,
  notificationSettingsLoading,
  theme,
  onSelectProject,
  onCreateProject,
  onUpdateProject,
  onDeleteProject,
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
  onUpdateNotificationSettings,
  onTestNotificationSettings,
  onLoadWorkspaceTree,
  onReadWorkspaceFile,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onMessageSent,
  onToggleTheme,
  onLogout
}: ShellProps) {
  const [agentPanelOpen, setAgentPanelOpen] = useState(false);
  const [focusedAgentID, setFocusedAgentID] = useState("");
  const [membersPanelOpen, setMembersPanelOpen] = useState(false);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [mobileAgentPanelOpen, setMobileAgentPanelOpen] = useState(false);
  const [mobileMembersPanelOpen, setMobileMembersPanelOpen] = useState(false);
  const [isMobileLayout, setIsMobileLayout] = useState(() =>
    typeof window !== "undefined"
      ? window.matchMedia("(max-width: 767px)").matches
      : false
  );
  const [projectName, setProjectName] = useState("");
  const [projectDraftOpen, setProjectDraftOpen] = useState(false);
  const [projectEditOpen, setProjectEditOpen] = useState(false);
  const [projectEditName, setProjectEditName] = useState("");
  const [projectEditWorkspacePath, setProjectEditWorkspacePath] = useState("");
  const [projectEditEmoji, setProjectEditEmoji] = useState("");
  const [projectEditColor, setProjectEditColor] = useState("");
  const [projectEditError, setProjectEditError] = useState<string | null>(null);
  const [projectEditPending, setProjectEditPending] = useState(false);
  const [accountSettingsOpen, setAccountSettingsOpen] = useState(false);
  const [channelDraftOpen, setChannelDraftOpen] = useState(false);
  const [channelName, setChannelName] = useState("");
  const [channelType, setChannelType] = useState<Channel["type"]>("text");
  const [agentDraftOpen, setAgentDraftOpen] = useState(false);
  const [newAgentName, setNewAgentName] = useState("");
  const [newAgentDescription, setNewAgentDescription] = useState("");
  const [newAgentHandle, setNewAgentHandle] = useState("");
  const [newAgentKind, setNewAgentKind] = useState("fake");
  const [newAgentModel, setNewAgentModel] = useState("");
  const [newAgentEffort, setNewAgentEffort] = useState("");
  const [newAgentFastMode, setNewAgentFastMode] = useState(false);
  const [newAgentYoloMode, setNewAgentYoloMode] = useState(false);
  const [newAgentEmoji, setNewAgentEmoji] = useState("");
  const [newAgentColor, setNewAgentColor] = useState("");
  const [newAgentError, setNewAgentError] = useState<string | null>(null);
  const [creatingAgent, setCreatingAgent] = useState(false);
  const [threadEditOpen, setThreadEditOpen] = useState(false);
  const [threadTitleDraft, setThreadTitleDraft] = useState("");
  const [threadActionError, setThreadActionError] = useState<string | null>(null);
  const [threadActionPending, setThreadActionPending] = useState(false);
  const [browserPermission, setBrowserPermission] = useState<BrowserNotificationPermission>(() =>
    browserNotificationPermission()
  );
  const [webhookEnabled, setWebhookEnabled] = useState(false);
  const [webhookURL, setWebhookURL] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [notificationActionError, setNotificationActionError] = useState<string | null>(null);
  const [notificationActionStatus, setNotificationActionStatus] = useState<string | null>(null);
  const [notificationSavePending, setNotificationSavePending] = useState(false);
  const [notificationTestPending, setNotificationTestPending] = useState(false);
  const boundAgents = conversationContext?.agents ?? channelAgents;
  const activeAgents = useMemo(() => agents.filter((agent) => agent.enabled), [agents]);
  const selectedAgent =
    agents.find((agent) => agent.id === focusedAgentID) ?? boundAgents[0]?.agent ?? activeAgents[0];
  const activeThread = conversationContext?.thread;

  useEffect(() => {
    const media = window.matchMedia("(max-width: 767px)");
    const update = () => setIsMobileLayout(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    if (isMobileLayout) {
      setAgentPanelOpen(false);
      setMembersPanelOpen(false);
    } else {
      setMobileNavOpen(false);
      setMobileAgentPanelOpen(false);
      setMobileMembersPanelOpen(false);
    }
  }, [isMobileLayout]);

  useEffect(() => {
    setAgentPanelOpen(false);
    setFocusedAgentID("");
  }, [selectedChannel?.id, activeConversation?.id]);

  useEffect(() => {
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setFocusedAgentID("");
  }, [selectedChannel?.id, activeConversation?.id]);

  useEffect(() => {
    setThreadTitleDraft(activeThread?.title ?? "");
    setThreadActionError(null);
    setThreadEditOpen(false);
  }, [activeThread?.id]);

  useEffect(() => {
    if (project) {
      setProjectEditName(project.name);
      setProjectEditWorkspacePath(projectWorkspace?.path ?? "");
      const avatar = getProjectAvatar(project.id);
      setProjectEditEmoji(avatar?.emoji ?? "");
      setProjectEditColor(avatar?.color ?? "");
    }
    setProjectEditError(null);
    setProjectEditOpen(false);
  }, [project?.id]);

  useEffect(() => {
    if (!projectEditOpen) {
      setProjectEditName(project?.name ?? "");
      setProjectEditWorkspacePath(projectWorkspace?.path ?? "");
    }
  }, [project?.name, projectWorkspace?.path, projectEditOpen]);

  useEffect(() => {
    if (!accountSettingsOpen) {
      syncNotificationDraft();
    }
  }, [
    accountSettingsOpen,
    notificationSettings?.organization_id,
    notificationSettings?.webhook_enabled,
    notificationSettings?.webhook_url
  ]);

  useEffect(() => {
    if (projectEditOpen && !projectEditWorkspacePath && projectWorkspace?.path) {
      setProjectEditWorkspacePath(projectWorkspace.path);
    }
  }, [projectEditOpen, projectEditWorkspacePath, projectWorkspace?.path]);

  async function submitProject() {
    const name = projectName.trim();
    if (!name) return;
    const created = await onCreateProject(name);
    setProjectName("");
    setProjectDraftOpen(false);
    onSelectProject(created.id);
  }

  async function submitProjectEdit() {
    if (!project) return;
    const name = projectEditName.trim();
    const workspacePath = projectEditWorkspacePath.trim();
    if (!name || !workspacePath) return;
    setProjectEditError(null);
    setProjectEditPending(true);
    try {
      await onUpdateProject(project.id, {
        name,
        workspace_path: workspacePath,
      });
      setProjectAvatar(
        project.id,
        projectEditEmoji.trim()
          ? {
              emoji: projectEditEmoji.trim(),
              color: projectEditColor || "bg-primary",
            }
          : null
      );
      setProjectEditOpen(false);
    } catch (err) {
      setProjectEditError(err instanceof Error ? err.message : "Update project failed");
    } finally {
      setProjectEditPending(false);
    }
  }

  async function deleteActiveProject() {
    if (!project) return;
    if (!window.confirm(`Delete project "${project.name}"?`)) return;
    setProjectEditError(null);
    setProjectEditPending(true);
    try {
      await onDeleteProject(project);
      setProjectEditOpen(false);
    } catch (err) {
      setProjectEditError(err instanceof Error ? err.message : "Delete project failed");
    } finally {
      setProjectEditPending(false);
    }
  }

  function openProjectSettings() {
    if (!project) return;
    blurActiveElement();
    setProjectEditName(project.name);
    setProjectEditWorkspacePath(projectWorkspace?.path ?? "");
    const avatar = getProjectAvatar(project.id);
    setProjectEditEmoji(avatar?.emoji ?? "");
    setProjectEditColor(avatar?.color ?? "");
    setProjectEditError(null);
    setProjectEditOpen(true);
  }

  function openCreateProject() {
    blurActiveElement();
    setProjectDraftOpen(true);
  }

  function openAccountSettings() {
    blurActiveElement();
    setBrowserPermission(browserNotificationPermission());
    syncNotificationDraft();
    setAccountSettingsOpen(true);
  }

  function syncNotificationDraft() {
    setWebhookEnabled(notificationSettings?.webhook_enabled ?? false);
    setWebhookURL(notificationSettings?.webhook_url ?? "");
    setWebhookSecret("");
    setNotificationActionError(null);
    setNotificationActionStatus(null);
  }

  async function enableBrowserNotifications() {
    const permission = await requestBrowserNotificationPermission();
    setBrowserPermission(permission);
  }

  async function saveNotificationSettings() {
    setNotificationActionError(null);
    setNotificationActionStatus(null);
    setNotificationSavePending(true);
    try {
      await onUpdateNotificationSettings(notificationSettingsPayload());
      setWebhookSecret("");
      setNotificationActionStatus("Saved");
    } catch (err) {
      setNotificationActionError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setNotificationSavePending(false);
    }
  }

  async function sendTestWebhook() {
    setNotificationActionError(null);
    setNotificationActionStatus(null);
    setNotificationTestPending(true);
    try {
      await onUpdateNotificationSettings(notificationSettingsPayload());
      setWebhookSecret("");
      await onTestNotificationSettings();
      setNotificationActionStatus("Test delivered");
    } catch (err) {
      setNotificationActionError(err instanceof Error ? err.message : "Test failed");
    } finally {
      setNotificationTestPending(false);
    }
  }

  function notificationSettingsPayload(): {
    webhook_enabled: boolean;
    webhook_url: string;
    webhook_secret?: string;
  } {
    const secret = webhookSecret.trim();
    return {
      webhook_enabled: webhookEnabled,
      webhook_url: webhookURL.trim(),
      ...(secret ? { webhook_secret: secret } : {})
    };
  }

  function selectMobileProject(projectID: string) {
    setMobileNavOpen(false);
    onSelectProject(projectID);
  }

  function selectMobileChannel(channel: Channel) {
    setMobileNavOpen(false);
    onSelectChannel(channel);
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
        description: newAgentDescription.trim() || undefined,
        handle: newAgentHandle || undefined,
        kind: newAgentKind,
        model: newAgentModel || undefined,
        effort: newAgentEffort || undefined,
        fast_mode: newAgentFastMode,
        yolo_mode: newAgentYoloMode,
      });
      if (newAgentEmoji) {
        setAgentAvatar(created.id, { emoji: newAgentEmoji, color: newAgentColor || agentKindColor(newAgentKind) });
      }
      if (selectedChannel) {
        await onSaveChannelAgents([
          ...boundAgents
            .filter((item) => item.agent.id !== created.id)
            .map((item) => ({
              agent_id: item.agent.id,
              run_workspace_id: item.binding.run_workspace_id || undefined,
            })),
          { agent_id: created.id },
        ]);
      }
      setNewAgentName("");
      setNewAgentDescription("");
      setNewAgentHandle("");
      setNewAgentKind("fake");
      setNewAgentModel("");
      setNewAgentEffort("");
      setNewAgentFastMode(false);
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
    <div className="flex h-dvh w-screen overflow-hidden select-none" data-testid="agentx-shell">
      {isMobileLayout ? (
      <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="mobile-shell">
        <div className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-2">
          <Button
            variant="ghost"
            size="icon"
            className="h-11 w-11"
            title="Navigation"
            aria-label="Navigation"
            onClick={() => setMobileNavOpen(true)}
          >
            <Menu className="h-5 w-5" />
          </Button>
          {selectedChannel?.type === "thread" && activeThread ? (
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11"
              title="Back to posts"
              aria-label="Back to posts"
              onClick={() => onSelectChannel(selectedChannel)}
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
          ) : null}
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-sm font-semibold">{title}</h1>
            <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-11 w-11"
            title="Members"
            aria-label="Members"
            onClick={() => setMobileMembersPanelOpen(true)}
          >
            <UserRound className="h-5 w-5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-11 w-11"
            title="Agent settings"
            aria-label="Agent settings"
            onClick={() => setMobileAgentPanelOpen(true)}
          >
            <Settings className="h-5 w-5" />
          </Button>
        </div>

        {activeThread && (
          <div className="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
            {activeConversation && (
              <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                <Activity className="h-3.5 w-3.5 shrink-0" />
                {streaming.length > 0 ? "running" : "ready"}
              </span>
            )}
            <div className="ml-auto flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                className="h-9 w-9"
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
                className="h-9 w-9 text-muted-foreground hover:text-destructive"
                title="Delete post"
                aria-label="Delete post"
                disabled={threadActionPending}
                onClick={deleteActiveThread}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}

        <ConversationPanel
          selectedChannel={selectedChannel}
          activeThread={activeThread}
          threads={threads}
          messages={messages}
          messagesLoading={messagesLoading}
          olderMessagesLoading={olderMessagesLoading}
          hasOlderMessages={hasOlderMessages}
          streaming={streaming}
          boundAgents={boundAgents}
          composerConversation={composerConversation}
          onSelectThread={onSelectThread}
          onCreateThread={onCreateThread}
          onUpdateThread={onUpdateThread}
          onDeleteThread={onDeleteThread}
          onUpdateMessage={onUpdateMessage}
          onDeleteMessage={onDeleteMessage}
          onLoadOlderMessages={onLoadOlderMessages}
          onMessageSent={onMessageSent}
        />

        <Dialog open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
          <DialogContent
            showCloseButton={false}
            className="left-0 top-0 min-w-0 !h-svh !w-[100svw] !max-w-[100svw] !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-l-0 p-0 sm:!w-[24rem] sm:!max-w-sm"
          >
            <div className="flex h-full min-h-0 min-w-0 flex-col overflow-x-hidden bg-sidebar">
              <div className="flex h-14 min-w-0 shrink-0 items-center justify-between gap-3 border-b border-border px-4">
                <DialogHeader className="min-w-0 gap-0 text-left">
                  <DialogTitle className="truncate">Navigation</DialogTitle>
                  <DialogDescription className="truncate">{project?.name ?? "No project"}</DialogDescription>
                </DialogHeader>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10"
                  title="Close navigation"
                  aria-label="Close navigation"
                  onClick={() => setMobileNavOpen(false)}
                >
                  <X className="h-5 w-5" />
                </Button>
              </div>

              <ScrollArea
                className="min-h-0 min-w-0 flex-1"
                viewportClassName="max-w-full overflow-x-hidden"
                data-testid="mobile-nav-scroll"
              >
                <div className="min-w-0 max-w-full space-y-5 px-3 py-4">
                  <section aria-label="Projects" className="min-w-0 max-w-full space-y-1">
                    <div className="px-1 text-xs font-semibold uppercase text-muted-foreground">
                      Projects
                    </div>
                    {projects.map((item) => {
                      const avatar = getProjectAvatar(item.id);
                      const isSelected = item.id === project?.id;
                      return (
                        <button
                          key={item.id}
                          className={cn(
                            "flex min-h-11 min-w-0 max-w-full w-full items-center gap-3 overflow-hidden rounded-md px-2 text-left text-sm transition-colors",
                            isSelected
                              ? "bg-accent text-accent-foreground"
                              : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                          )}
                          aria-label={item.name}
                          onClick={() => selectMobileProject(item.id)}
                        >
                          <span
                            className={cn(
                              "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-xs font-semibold",
                              avatar?.emoji
                                ? cn("text-white", avatar.color || "bg-primary")
                                : "bg-secondary text-secondary-foreground"
                            )}
                          >
                            {avatar?.emoji ? avatar.emoji : initials(item.name)}
                          </span>
                          <span className="block min-w-0 max-w-[calc(100svw-8rem)] flex-1 truncate">{item.name}</span>
                        </button>
                      );
                    })}
                    <button
                      className="flex min-h-11 min-w-0 max-w-full w-full items-center gap-3 overflow-hidden rounded-md px-2 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                      title="Create project"
                      aria-label="Create project"
                      onClick={() => {
                        setMobileNavOpen(false);
                        openCreateProject();
                      }}
                    >
                      <Plus className="h-4 w-4 shrink-0" />
                      <span className="min-w-0 truncate">Create project</span>
                    </button>
                  </section>

                  <ChannelList
                    channels={channels}
                    selectedChannelID={selectedChannel?.id}
                    onSelect={selectMobileChannel}
                    onCreate={() => {
                      setMobileNavOpen(false);
                      setChannelDraftOpen(true);
                    }}
                    onUpdate={onUpdateChannel}
                    onDelete={onDeleteChannel}
                  />

                  <AgentsSidebar
                    agents={activeAgents}
                    boundAgents={boundAgents}
                    contextLoading={contextLoading}
                    onOpenPanel={(agentID) => {
                      if (agentID) setFocusedAgentID(agentID);
                      setMobileNavOpen(false);
                      setMobileAgentPanelOpen(true);
                    }}
                    onCreateAgent={() => {
                      setMobileNavOpen(false);
                      setAgentDraftOpen(true);
                    }}
                  />
                </div>
              </ScrollArea>

              <div
                className="min-w-0 shrink-0 border-t border-border px-3 py-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))]"
                data-testid="mobile-nav-footer"
              >
                <div className="mb-2 flex min-w-0 items-center gap-3">
                  <Avatar className="h-9 w-9">
                    <AvatarFallback className="bg-primary text-xs text-primary-foreground">
                      {initials(user.display_name)}
                    </AvatarFallback>
                  </Avatar>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{user.display_name}</p>
                    <p className="truncate text-xs text-muted-foreground">{organization?.name ?? "online"}</p>
                  </div>
                </div>
                <div className="grid min-w-0 grid-cols-3 gap-2">
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-10 min-w-0 w-full"
                    title="User settings"
                    aria-label="User settings"
                    onClick={() => {
                      setMobileNavOpen(false);
                      openAccountSettings();
                    }}
                  >
                    <Settings className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-10 min-w-0 w-full"
                    title={theme === "dark" ? "Light mode" : "Dark mode"}
                    aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
                    onClick={onToggleTheme}
                  >
                    {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-10 min-w-0 w-full"
                    title="Log out"
                    aria-label="Log out"
                    onClick={onLogout}
                  >
                    <LogOut className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
          </DialogContent>
        </Dialog>

        <Dialog open={mobileMembersPanelOpen} onOpenChange={setMobileMembersPanelOpen}>
          <DialogContent
            showCloseButton={false}
            className="left-auto right-0 top-0 !h-svh w-[92vw] max-w-sm !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-r-0 p-0 sm:max-w-sm"
          >
            <DialogTitle className="sr-only">Members</DialogTitle>
            <DialogDescription className="sr-only">Manage channel members.</DialogDescription>
            <MembersPanel
              agents={activeAgents}
              boundAgents={boundAgents}
              projectWorkspace={projectWorkspace}
              selectedChannel={selectedChannel}
              onSaveChannelAgents={onSaveChannelAgents}
              onClose={() => setMobileMembersPanelOpen(false)}
            />
          </DialogContent>
        </Dialog>

        <Dialog open={mobileAgentPanelOpen} onOpenChange={setMobileAgentPanelOpen}>
          <DialogContent
            showCloseButton={false}
            className="left-auto right-0 top-0 !h-svh w-[96vw] max-w-md !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-r-0 p-0 sm:max-w-md"
          >
            <DialogTitle className="sr-only">Agent settings</DialogTitle>
            <DialogDescription className="sr-only">Manage agent settings and workspace files.</DialogDescription>
            <AgentDetailsPanel
              selectedChannel={selectedChannel}
              projectWorkspace={projectWorkspace}
              agents={activeAgents}
              boundAgents={boundAgents}
              selectedAgent={selectedAgent}
              onSaveChannelAgents={onSaveChannelAgents}
              onUpdateAgent={onUpdateAgent}
              onDeleteAgent={onDeleteAgent}
              onLoadWorkspaceTree={onLoadWorkspaceTree}
              onReadWorkspaceFile={onReadWorkspaceFile}
              onWriteWorkspaceFile={onWriteWorkspaceFile}
              onDeleteWorkspaceFile={onDeleteWorkspaceFile}
              onCreateAgentModal={() => setAgentDraftOpen(true)}
              onClose={() => setMobileAgentPanelOpen(false)}
            />
          </DialogContent>
        </Dialog>
      </div>
      ) : null}

      {!isMobileLayout ? (
      <div className="flex h-full min-h-0 min-w-0 flex-1" data-testid="desktop-shell">
      {/* Project Rail */}
      <TooltipProvider delayDuration={0}>
        <div className="flex h-full w-[72px] flex-col items-center gap-2 border-r border-sidebar-border/70 bg-sidebar py-3">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-primary text-primary-foreground font-bold text-lg"
              >
                AX
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">AgentX</TooltipContent>
          </Tooltip>

          <div className="mx-auto h-0.5 w-8 shrink-0 rounded-full bg-border" />

          <ScrollArea className="min-h-0 w-full flex-1">
            <div className="flex flex-col items-center gap-2">
              {projects.map((item) => {
                const avatar = getProjectAvatar(item.id);
                const isSelected = item.id === project?.id;
                return (
                  <Tooltip key={item.id}>
                    <TooltipTrigger asChild>
                      <button
                        className={cn(
                          "relative flex h-12 w-12 items-center justify-center rounded-2xl transition-all hover:rounded-xl",
                          avatar?.emoji
                            ? cn("text-white", avatar.color || "bg-primary")
                            : "bg-secondary text-secondary-foreground hover:bg-primary hover:text-primary-foreground",
                          isSelected &&
                            (avatar?.emoji
                              ? "rounded-xl ring-2 ring-ring ring-offset-2 ring-offset-sidebar"
                              : "rounded-xl bg-primary text-primary-foreground")
                        )}
                        title={item.name}
                        aria-label={item.name}
                        onClick={() => onSelectProject(item.id)}
                      >
                        {avatar?.emoji ? (
                          <span className="text-xl">{avatar.emoji}</span>
                        ) : (
                          <span className="text-lg font-semibold">{initials(item.name)}</span>
                        )}
                        {isSelected && (
                          <div className="absolute -left-3 h-10 w-1 rounded-r-full bg-foreground" />
                        )}
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="right">{item.name}</TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          </ScrollArea>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className={cn(
                  "flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-secondary text-muted-foreground transition-all hover:rounded-xl hover:bg-green-600 hover:text-white",
                  projectDraftOpen && "rounded-xl bg-green-600 text-white"
                )}
                title="Create project"
                aria-label="Create project"
                onClick={openCreateProject}
              >
                <Plus className="h-5 w-5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">Create project</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
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
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
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
          <div className="flex h-full min-h-0 flex-col bg-sidebar">
            {/* Workspace Header */}
            <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
              <h2 className="truncate text-base font-semibold">
                {project?.name ?? "No project"}
              </h2>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                title="Project settings"
                aria-label="Project settings"
                disabled={!project}
                onClick={openProjectSettings}
              >
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
                  onOpenPanel={(agentID) => {
                    if (agentID) setFocusedAgentID(agentID);
                    setMembersPanelOpen(false);
                    setAgentPanelOpen(true);
                  }}
                  onCreateAgent={() => setAgentDraftOpen(true)}
                />
              </div>
            </ScrollArea>

            {/* User Info */}
            <div className="flex shrink-0 items-center gap-2 border-t border-border bg-sidebar p-2">
              <Avatar className="h-8 w-8">
                <AvatarFallback className="bg-primary text-primary-foreground text-xs">
                  {initials(user.display_name)}
                </AvatarFallback>
              </Avatar>
              <div className="flex-1 truncate">
                <p className="text-sm font-medium">{user.display_name}</p>
                <p className="text-xs text-muted-foreground">online</p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                title="User settings"
                aria-label="User settings"
                onClick={openAccountSettings}
              >
                <Settings className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </ResizablePanel>

        <ResizableHandle withHandle />

        {/* Message Area */}
        <ResizablePanel defaultSize={agentPanelOpen ? 55 : membersPanelOpen ? 62 : 82}>
          <div className="flex h-full min-h-0 flex-1 flex-col bg-background">
            {/* Channel Header */}
            <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
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
                  className={cn("h-8 w-8", agentPanelOpen && "bg-accent")}
                  title="Agent settings"
                  aria-label="Agent settings"
                  onClick={() => {
                    setMembersPanelOpen(false);
                    setAgentPanelOpen((open) => !open);
                  }}
                >
                  <Settings className="h-4 w-4" />
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
                projectWorkspace={projectWorkspace}
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
                projectWorkspace={projectWorkspace}
                agents={activeAgents}
                boundAgents={boundAgents}
                selectedAgent={selectedAgent}
                onSaveChannelAgents={onSaveChannelAgents}
                onUpdateAgent={onUpdateAgent}
                onDeleteAgent={onDeleteAgent}
                onLoadWorkspaceTree={onLoadWorkspaceTree}
                onReadWorkspaceFile={onReadWorkspaceFile}
                onWriteWorkspaceFile={onWriteWorkspaceFile}
                onDeleteWorkspaceFile={onDeleteWorkspaceFile}
                onCreateAgentModal={() => setAgentDraftOpen(true)}
                onClose={() => setAgentPanelOpen(false)}
              />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
      </div>
      ) : null}

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

      {/* Account Settings Modal */}
      <Dialog open={accountSettingsOpen} onOpenChange={setAccountSettingsOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>User settings</DialogTitle>
            <DialogDescription>Session and workspace details.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="grid gap-3 rounded-md border border-border p-3 text-sm">
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">User</span>
                <span className="truncate font-medium">{user.display_name}</span>
              </div>
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">Organization</span>
                <span className="truncate font-medium">{organization?.name ?? "None"}</span>
              </div>
              <div className="flex items-center justify-between gap-4">
                <span className="text-muted-foreground">Project</span>
                <span className="truncate font-medium">{project?.name ?? "None"}</span>
              </div>
            </div>
            <div className="grid gap-3 rounded-md border border-border p-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h3 className="text-sm font-medium">Browser notifications</h3>
                  <p className="text-xs text-muted-foreground">{browserPermissionLabel(browserPermission)}</p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={enableBrowserNotifications}
                  disabled={browserPermission === "granted" || browserPermission === "denied" || browserPermission === "unsupported"}
                >
                  Enable
                </Button>
              </div>
            </div>
            <div className="grid gap-3 rounded-md border border-border p-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h3 className="text-sm font-medium">Webhook</h3>
                  <p className="text-xs text-muted-foreground">
                    {notificationSettings?.webhook_secret_configured ? "Secret configured" : "No secret configured"}
                  </p>
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={webhookEnabled}
                    onChange={(event) => setWebhookEnabled(event.currentTarget.checked)}
                    disabled={notificationSettingsLoading}
                    aria-label="Enable webhook"
                  />
                  Enabled
                </label>
              </div>
              <div className="space-y-2">
                <Label htmlFor="webhook-url">URL</Label>
                <Input
                  id="webhook-url"
                  value={webhookURL}
                  onChange={(event) => setWebhookURL(event.target.value)}
                  placeholder="https://example.com/agentx/${title}/${body}"
                  disabled={notificationSettingsLoading}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="webhook-secret">Secret</Label>
                <Input
                  id="webhook-secret"
                  value={webhookSecret}
                  onChange={(event) => setWebhookSecret(event.target.value)}
                  placeholder={notificationSettings?.webhook_secret_configured ? "Leave blank to keep current secret" : "Optional signing secret"}
                  disabled={notificationSettingsLoading}
                  type="password"
                />
              </div>
              {(notificationActionError || notificationActionStatus) && (
                <p className={cn("text-sm", notificationActionError ? "text-destructive" : "text-muted-foreground")}>
                  {notificationActionError ?? notificationActionStatus}
                </p>
              )}
              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={sendTestWebhook}
                  disabled={notificationSettingsLoading || notificationTestPending || !webhookURL.trim()}
                >
                  <Send className="h-4 w-4" />
                  Test
                </Button>
                <Button
                  type="button"
                  onClick={saveNotificationSettings}
                  disabled={notificationSettingsLoading || notificationSavePending || (webhookEnabled && !webhookURL.trim())}
                >
                  <Save className="h-4 w-4" />
                  Save
                </Button>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAccountSettingsOpen(false)}>
              Close
            </Button>
            <Button variant="destructive" onClick={onLogout}>
              <LogOut className="h-4 w-4" />
              Log out
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Project Modal */}
      <Dialog open={projectEditOpen} onOpenChange={setProjectEditOpen}>
        <DialogContent onOpenAutoFocus={(event) => event.preventDefault()}>
          <DialogHeader>
            <DialogTitle>Project settings</DialogTitle>
            <DialogDescription>Update or delete this project.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="flex items-center gap-4">
              <div
                className={cn(
                  "flex h-14 w-14 shrink-0 items-center justify-center rounded-xl text-white",
                  projectEditEmoji ? projectEditColor || "bg-primary" : "bg-primary"
                )}
              >
                {projectEditEmoji ? (
                  <span className="text-2xl">{projectEditEmoji}</span>
                ) : (
                  <span className="text-lg font-semibold">{initials(projectEditName || project?.name || "Project")}</span>
                )}
              </div>
              <div className="min-w-0 flex-1 space-y-2">
                <Input
                  value={projectEditEmoji}
                  onChange={(e) => setProjectEditEmoji(e.target.value)}
                  placeholder="Icon emoji"
                  aria-label="Project icon"
                />
                <div className="flex flex-wrap gap-1.5">
                  {AVATAR_COLORS.map((color) => (
                    <button
                      key={color}
                      className={cn(
                        "h-5 w-5 rounded-full transition-all",
                        color,
                        projectEditColor === color
                          ? "ring-2 ring-ring ring-offset-1 ring-offset-background"
                          : "opacity-60 hover:opacity-100"
                      )}
                      aria-label="Project color"
                      onClick={() => setProjectEditColor(color)}
                      type="button"
                    />
                  ))}
                  {projectEditEmoji && (
                    <button
                      className="h-5 rounded-full border border-border px-2 text-[10px] text-muted-foreground hover:text-foreground"
                      onClick={() => {
                        setProjectEditEmoji("");
                        setProjectEditColor("");
                      }}
                      type="button"
                    >
                      Reset
                    </button>
                  )}
                </div>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="project-edit-name">Project name</Label>
              <Input
                id="project-edit-name"
                value={projectEditName}
                onChange={(e) => setProjectEditName(e.target.value)}
                aria-label="Project name"
                onKeyDown={(e) => { if (e.key === "Enter") void submitProjectEdit(); }}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="project-edit-workspace">Workspace path</Label>
              <Input
                id="project-edit-workspace"
                value={projectEditWorkspacePath}
                onChange={(e) => setProjectEditWorkspacePath(e.target.value)}
                placeholder={projectWorkspace ? "" : "Loading workspace..."}
                aria-label="Workspace path"
                onKeyDown={(e) => { if (e.key === "Enter") void submitProjectEdit(); }}
              />
            </div>
            {projectEditError && <p className="text-sm text-destructive">{projectEditError}</p>}
          </div>
          <DialogFooter className="sm:justify-between">
            <Button
              variant="destructive"
              onClick={deleteActiveProject}
              disabled={!project || projectEditPending}
            >
              <Trash2 className="h-4 w-4" />
              Delete
            </Button>
            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              variant="outline"
              onClick={() => setProjectEditOpen(false)}
              disabled={projectEditPending}
            >
              Cancel
            </Button>
            <Button
              onClick={submitProjectEdit}
              disabled={!projectEditName.trim() || !projectEditWorkspacePath.trim() || projectEditPending}
            >
              Save
            </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Project Modal */}
      <Dialog open={projectDraftOpen} onOpenChange={setProjectDraftOpen}>
        <DialogContent onOpenAutoFocus={(event) => event.preventDefault()}>
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
              <Select
                id="channel-type"
                value={channelType}
                onChange={(e) => setChannelType(e.target.value as Channel["type"])}
                aria-label="Channel type"
              >
                <option value="text">Text</option>
                <option value="thread">Forum</option>
              </Select>
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
              <Label htmlFor="new-agent-description">Description</Label>
              <Textarea
                id="new-agent-description"
                value={newAgentDescription}
                onChange={(e) => setNewAgentDescription(e.target.value)}
                placeholder="What this agent is responsible for"
                aria-label="New agent description"
                rows={3}
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
                <Select
                  id="new-agent-runtime"
                  value={newAgentKind}
                  onChange={(e) => setNewAgentKind(e.target.value)}
                  aria-label="New agent runtime"
                >
                  <option value="fake">Fake</option>
                  <option value="codex">Codex</option>
                  <option value="claude">Claude</option>
                </Select>
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
              <div className="space-y-2">
                <Label htmlFor="new-agent-effort">Effort</Label>
                <Select
                  id="new-agent-effort"
                  value={newAgentEffort}
                  onChange={(e) => setNewAgentEffort(e.target.value)}
                  aria-label="New agent effort"
                >
                  <option value="">Default</option>
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                </Select>
              </div>
            </div>
            <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
              <Checkbox
                checked={newAgentFastMode}
                onChange={(e) => setNewAgentFastMode(e.target.checked)}
                aria-label="New agent fast mode"
              />
              Fast mode
            </label>
            <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
              <Checkbox
                checked={newAgentYoloMode}
                onChange={(e) => setNewAgentYoloMode(e.target.checked)}
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

function ConversationPanel({
  selectedChannel,
  activeThread,
  threads,
  messages,
  messagesLoading,
  olderMessagesLoading,
  hasOlderMessages,
  streaming,
  boundAgents,
  composerConversation,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onMessageSent,
}: {
  selectedChannel?: Channel;
  activeThread?: Thread;
  threads: Thread[];
  messages: Message[];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  boundAgents: ConversationAgentContext[];
  composerConversation?: ComposerConversation;
  onSelectThread: ShellProps["onSelectThread"];
  onCreateThread: ShellProps["onCreateThread"];
  onUpdateThread: ShellProps["onUpdateThread"];
  onDeleteThread: ShellProps["onDeleteThread"];
  onUpdateMessage: ShellProps["onUpdateMessage"];
  onDeleteMessage: ShellProps["onDeleteMessage"];
  onLoadOlderMessages: ShellProps["onLoadOlderMessages"];
  onMessageSent: ShellProps["onMessageSent"];
}) {
  if (selectedChannel?.type === "thread" && !activeThread) {
    return (
      <ThreadForum
        threads={threads}
        onSelectThread={onSelectThread}
        onCreateThread={onCreateThread}
        onUpdateThread={onUpdateThread}
        onDeleteThread={onDeleteThread}
      />
    );
  }

  return (
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
  onOpenPanel: (agentID?: string) => void;
  onCreateAgent: () => void;
}) {
  const [open, setOpen] = useState(true);

  return (
    <section aria-label="Bound agent" className="min-w-0 max-w-full overflow-hidden">
      <Collapsible open={open} onOpenChange={setOpen} className="mt-4 min-w-0 max-w-full overflow-hidden">
        <CollapsibleTrigger asChild>
          <button className="flex min-w-0 max-w-full w-full items-center gap-1 px-1 py-1 text-xs font-semibold uppercase text-muted-foreground hover:text-foreground">
            {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            Agents
            {contextLoading && <span className="ml-1 h-1.5 w-1.5 rounded-full bg-yellow-500 animate-pulse" />}
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="min-w-0 max-w-full space-y-0.5 overflow-hidden">
          {boundAgents.map((item) => (
            <button
              key={item.agent.id}
              className="group flex min-h-10 min-w-0 max-w-full w-full items-center gap-2 overflow-hidden rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-foreground md:min-h-0"
              aria-label={item.agent.name}
              onClick={() => onOpenPanel(item.agent.id)}
            >
              <div className="relative">
                <AgentAvatar agentID={item.agent.id} kind={item.agent.kind} size="sm" className="h-5 w-5" />
                <div className={cn(
                  "absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border border-card",
                  item.agent.enabled ? "bg-green-500" : "bg-gray-500"
                )} />
              </div>
              <span className="min-w-0 max-w-[calc(100svw-8rem)] truncate">{item.agent.name}</span>
              <Settings className="ml-auto h-3 w-3 opacity-0 group-hover:opacity-100" />
            </button>
          ))}
          {boundAgents.length === 0 && (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">Unbound</p>
          )}
          <button
              className="flex min-h-10 min-w-0 max-w-full w-full items-center gap-2 overflow-hidden rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground md:min-h-0"
            onClick={onCreateAgent}
          >
            <Plus className="h-4 w-4 shrink-0" />
            <span className="min-w-0 truncate">Create agent</span>
          </button>
          <button
            className="flex min-h-10 min-w-0 max-w-full w-full items-center gap-2 overflow-hidden rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground md:min-h-0"
            onClick={() => onOpenPanel()}
          >
            <Settings className="h-4 w-4 shrink-0" />
            <span className="min-w-0 truncate">Manage agents</span>
          </button>
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}

function MembersPanel({
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
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden p-3 md:p-4" aria-label="Threads">
      <ScrollArea className="min-h-0 flex-1">
        <div className="space-y-1">
          {threads.map((thread) => (
            <div
              key={thread.id}
              className="group flex min-h-10 w-full items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-accent/50 md:min-h-0"
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
                    <time className="hidden shrink-0 text-xs text-muted-foreground sm:block">{formatDate(thread.updated_at)}</time>
                  </button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-9 w-9 opacity-100 transition-opacity md:h-8 md:w-8 md:opacity-0 md:group-hover:opacity-100 focus:opacity-100"
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
                    className="h-9 w-9 text-muted-foreground opacity-100 transition-opacity hover:text-destructive md:h-8 md:w-8 md:opacity-0 md:group-hover:opacity-100 focus:opacity-100"
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

      <div className="mt-4 shrink-0 space-y-2 rounded-lg border border-border p-3">
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
  onClose
}: {
  selectedChannel?: Channel;
  projectWorkspace?: Workspace;
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
  onDeleteWorkspaceFile: ShellProps["onDeleteWorkspaceFile"];
  onCreateAgentModal: () => void;
  onClose: () => void;
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

function runWorkspaceOptions(
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

function defaultAgentInstructionPath(kind?: string): string {
  return kind === "claude" ? "CLAUDE.md" : "AGENTS.md";
}

function blurActiveElement() {
  const active = document.activeElement;
  if (active instanceof HTMLElement) {
    active.blur();
  }
}

function browserPermissionLabel(permission: BrowserNotificationPermission): string {
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

function getProjectAvatar(projectID: string): ProjectAvatarData | null {
  try {
    const raw = localStorage.getItem(projectAvatarStorageKey);
    if (!raw) return null;
    const map = JSON.parse(raw) as Record<string, ProjectAvatarData>;
    return map[projectID] ?? null;
  } catch {
    return null;
  }
}

function setProjectAvatar(projectID: string, data: ProjectAvatarData | null): void {
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
