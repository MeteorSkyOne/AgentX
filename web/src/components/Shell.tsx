import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  ArrowLeft,
  BarChart3,
  Bot,
  ChevronDown,
  FolderOpen,
  Hash,
  LogOut,
  Menu,
  Moon,
  Pencil,
  Plus,
  Rows3,
  Save,
  Send,
  Server as ServerIcon,
  Settings,
  Sun,
  Trash2,
  Upload,
  UserRound,
  X
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import { ChannelList } from "./ChannelList";
import type { Channel } from "../api/types";
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
import { Switch } from "@/components/ui/switch";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AVATAR_COLORS,
  agentKindColor,
  setAgentAvatar,
} from "./AgentAvatar";
import {
  browserNotificationPermission,
  requestBrowserNotificationPermission,
  type BrowserNotificationPermission,
} from "../notifications/browser";
import { AgentDetailsPanel } from "./shell/AgentDetailsPanel";
import { AgentsSidebar } from "./shell/AgentsSidebar";
import { uniqueAgents, uniqueConversationAgents } from "./shell/agentLists";
import { ConversationPanel } from "./shell/ConversationPanel";
import { MembersPanel } from "./shell/MembersPanel";
import { MetricsPanel } from "./shell/MetricsPanel";
import {
  useWorkspaceFileBrowser,
  WorkspaceFileEditorPane,
  WorkspaceFileTreePane,
} from "./WorkspaceFileBrowser";
import type { ShellProps } from "./shell/types";
import {
  AGENT_EFFORT_OPTIONS,
  agentToneColor,
  blurActiveElement,
  browserPermissionLabel,
  conversationActivityLabel,
  conversationSubtitle,
  conversationTitle,
  getProjectAvatar,
  initials,
  normalizeAgentHandle,
  setProjectAvatar,
} from "./shell/utils";

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
  connectionStatus,
  notificationSettings,
  notificationSettingsLoading,
  serverSettings,
  serverSettingsLoading,
  serverSettingsError,
  preferences,
  preferencesLoading,
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
  onUpdateServerSettings,
  onUpdateUserPreferences,
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
  const [projectFilesOpen, setProjectFilesOpen] = useState(false);
  const [mainView, setMainView] = useState<"chat" | "metrics">("chat");
  const [mobileProjectFilesView, setMobileProjectFilesView] = useState<"tree" | "editor">("tree");
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
  const [channelTeamMaxBatches, setChannelTeamMaxBatches] = useState("6");
  const [channelTeamMaxRuns, setChannelTeamMaxRuns] = useState("12");
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
  const [showTTFT, setShowTTFT] = useState(preferences.show_ttft);
  const [showTPS, setShowTPS] = useState(preferences.show_tps);
  const [notificationActionError, setNotificationActionError] = useState<string | null>(null);
  const [notificationActionStatus, setNotificationActionStatus] = useState<string | null>(null);
  const [notificationSavePending, setNotificationSavePending] = useState(false);
  const [notificationTestPending, setNotificationTestPending] = useState(false);
  const [serverListenIP, setServerListenIP] = useState("127.0.0.1");
  const [serverListenPort, setServerListenPort] = useState("8080");
  const [serverTLSEnabled, setServerTLSEnabled] = useState(false);
  const [serverTLSListenPort, setServerTLSListenPort] = useState("8443");
  const [serverTLSCertFile, setServerTLSCertFile] = useState("");
  const [serverTLSKeyFile, setServerTLSKeyFile] = useState("");
  const [serverTLSCertPEM, setServerTLSCertPEM] = useState("");
  const [serverTLSKeyPEM, setServerTLSKeyPEM] = useState("");
  const [serverSettingsPending, setServerSettingsPending] = useState(false);
  const [serverSettingsActionError, setServerSettingsActionError] = useState<string | null>(null);
  const [serverSettingsActionStatus, setServerSettingsActionStatus] = useState<string | null>(null);
  const [preferencesPending, setPreferencesPending] = useState(false);
  const [preferencesError, setPreferencesError] = useState<string | null>(null);
  const visibleAgents = useMemo(() => uniqueAgents(agents), [agents]);
  const boundAgents = useMemo(
    () => uniqueConversationAgents(conversationContext?.agents ?? channelAgents),
    [channelAgents, conversationContext?.agents]
  );
  const activeAgents = useMemo(() => visibleAgents.filter((agent) => agent.enabled), [visibleAgents]);
  const selectedAgent =
    visibleAgents.find((agent) => agent.id === focusedAgentID) ?? boundAgents[0]?.agent ?? activeAgents[0];
  const serverListenPortNumber = Number(serverListenPort);
  const serverListenPortValid =
    Number.isInteger(serverListenPortNumber) && serverListenPortNumber >= 1 && serverListenPortNumber <= 65535;
  const serverTLSListenPortNumber = Number(serverTLSListenPort);
  const serverTLSListenPortValid =
    Number.isInteger(serverTLSListenPortNumber) &&
    serverTLSListenPortNumber >= 1 &&
    serverTLSListenPortNumber <= 65535;
  const serverPortsConflict = serverTLSEnabled && serverListenPortNumber === serverTLSListenPortNumber;
  const serverTLSCertAvailable = Boolean(serverTLSCertFile.trim() || serverTLSCertPEM.trim());
  const serverTLSKeyAvailable = Boolean(serverTLSKeyFile.trim() || serverTLSKeyPEM.trim());
  const serverSettingsSaveDisabled =
    serverSettingsLoading ||
    serverSettingsPending ||
    !serverListenIP.trim() ||
    !serverListenPortValid ||
    !serverTLSListenPortValid ||
    (serverTLSEnabled && serverPortsConflict) ||
    (serverTLSEnabled && (!serverTLSCertAvailable || !serverTLSKeyAvailable));
  const activeThread = conversationContext?.thread;
  const projectFilesController = useWorkspaceFileBrowser({
    workspaceID: projectWorkspace?.id,
    workspacePath: projectWorkspace?.path,
    onLoadTree: onLoadWorkspaceTree,
    onReadFile: onReadWorkspaceFile,
    onWriteFile: onWriteWorkspaceFile,
    onDeleteFile: onDeleteWorkspaceFile,
  });
  const projectWorkspaceIDRef = useRef(projectWorkspace?.id);
  const projectLoadFileRef = useRef(projectFilesController.loadFile);
  const projectLoadTreeRef = useRef(projectFilesController.loadTree);
  projectWorkspaceIDRef.current = projectWorkspace?.id;
  projectLoadFileRef.current = projectFilesController.loadFile;
  projectLoadTreeRef.current = projectFilesController.loadTree;

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
    setMobileProjectFilesView("tree");
  }, [project?.id, projectWorkspace?.id]);

  useEffect(() => {
    setAgentPanelOpen(false);
    setFocusedAgentID("");
    setMainView("chat");
  }, [selectedChannel?.id, activeConversation?.id]);

  useEffect(() => {
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setFocusedAgentID("");
    setMainView("chat");
  }, [selectedChannel?.id, activeConversation?.id]);

  useEffect(() => {
    setMainView("chat");
  }, [project?.id]);

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
      syncServerSettingsDraft();
      syncPreferenceDraft();
    }
  }, [
    accountSettingsOpen,
    notificationSettings?.organization_id,
    notificationSettings?.webhook_enabled,
    notificationSettings?.webhook_url,
    serverSettings?.organization_id,
    serverSettings?.listen_ip,
    serverSettings?.listen_port,
    serverSettings?.tls.enabled,
    serverSettings?.tls.listen_port,
    serverSettings?.tls.cert_file,
    serverSettings?.tls.key_file,
    preferences.show_ttft,
    preferences.show_tps
  ]);

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
    syncServerSettingsDraft();
    setAccountSettingsOpen(true);
  }

  function syncNotificationDraft() {
    setWebhookEnabled(notificationSettings?.webhook_enabled ?? false);
    setWebhookURL(notificationSettings?.webhook_url ?? "");
    setWebhookSecret("");
    setNotificationActionError(null);
    setNotificationActionStatus(null);
  }

  function syncServerSettingsDraft() {
    setServerListenIP(serverSettings?.listen_ip ?? "127.0.0.1");
    setServerListenPort(String(serverSettings?.listen_port ?? 8080));
    setServerTLSEnabled(serverSettings?.tls.enabled ?? false);
    setServerTLSListenPort(String(serverSettings?.tls.listen_port ?? 8443));
    setServerTLSCertFile(serverSettings?.tls.cert_file ?? "");
    setServerTLSKeyFile(serverSettings?.tls.key_file ?? "");
    setServerTLSCertPEM("");
    setServerTLSKeyPEM("");
    setServerSettingsActionError(null);
    setServerSettingsActionStatus(null);
  }

  function syncPreferenceDraft() {
    setShowTTFT(preferences.show_ttft);
    setShowTPS(preferences.show_tps);
    setPreferencesError(null);
  }

  async function saveMetricDisplayPreferences(next: { show_ttft: boolean; show_tps: boolean }) {
    setShowTTFT(next.show_ttft);
    setShowTPS(next.show_tps);
    setPreferencesError(null);
    setPreferencesPending(true);
    try {
      await onUpdateUserPreferences(next);
    } catch (err) {
      setShowTTFT(preferences.show_ttft);
      setShowTPS(preferences.show_tps);
      setPreferencesError(err instanceof Error ? err.message : "Save preferences failed");
    } finally {
      setPreferencesPending(false);
    }
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

  async function saveServerSettings() {
    setServerSettingsActionError(null);
    setServerSettingsActionStatus(null);
    setServerSettingsPending(true);
    try {
      const updated = await onUpdateServerSettings(serverSettingsPayload());
      setServerListenIP(updated.listen_ip);
      setServerListenPort(String(updated.listen_port));
      setServerTLSEnabled(updated.tls.enabled);
      setServerTLSListenPort(String(updated.tls.listen_port));
      setServerTLSCertFile(updated.tls.cert_file);
      setServerTLSKeyFile(updated.tls.key_file);
      setServerTLSCertPEM("");
      setServerTLSKeyPEM("");
      setServerSettingsActionStatus(updated.restart_required ? "Saved. Restart required." : "Saved");
    } catch (err) {
      setServerSettingsActionError(err instanceof Error ? err.message : "Save server settings failed");
    } finally {
      setServerSettingsPending(false);
    }
  }

  function serverSettingsPayload() {
    const certPEM = serverTLSCertPEM.trim();
    const keyPEM = serverTLSKeyPEM.trim();
    return {
      listen_ip: serverListenIP.trim(),
      listen_port: Number(serverListenPort),
      tls: {
        enabled: serverTLSEnabled,
        listen_port: Number(serverTLSListenPort),
        cert_file: serverTLSCertFile.trim(),
        key_file: serverTLSKeyFile.trim(),
        ...(certPEM ? { cert_pem: certPEM } : {}),
        ...(keyPEM ? { key_pem: keyPEM } : {})
      }
    };
  }

  async function readServerPEMUpload(file: File, update: (value: string) => void) {
    setServerSettingsActionError(null);
    setServerSettingsActionStatus(null);
    try {
      update(await file.text());
    } catch (err) {
      setServerSettingsActionError(err instanceof Error ? err.message : "Read file failed");
    }
  }

  function openProjectFiles() {
    if (!projectWorkspace?.id) return;
    blurActiveElement();
    setMainView("chat");
    setAgentPanelOpen(false);
    setMembersPanelOpen(false);
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setMobileProjectFilesView("tree");
    setProjectFilesOpen(true);
    void projectFilesController.loadTree({ quiet: true });
  }

  const openWorkspacePath = useCallback((target: WorkspacePathTarget) => {
    if (!projectWorkspaceIDRef.current) return;
    blurActiveElement();
    setMainView("chat");
    setAgentPanelOpen(false);
    setMembersPanelOpen(false);
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setMobileProjectFilesView("editor");
    setProjectFilesOpen(true);
    void projectLoadFileRef.current(target.path, {
      position: target.lineNumber
        ? { lineNumber: target.lineNumber, column: target.column ?? 1 }
        : undefined,
    });
    void projectLoadTreeRef.current({ quiet: true });
  }, []);

  function openMetrics() {
    if (!project && !selectedChannel) return;
    blurActiveElement();
    setProjectFilesOpen(false);
    setMobileProjectFilesView("tree");
    setAgentPanelOpen(false);
    setMembersPanelOpen(false);
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setMainView("metrics");
  }

  function toggleProjectFiles() {
    if (projectFilesOpen) {
      blurActiveElement();
      setProjectFilesOpen(false);
      setMobileProjectFilesView("tree");
      setAgentPanelOpen(false);
      setMembersPanelOpen(false);
      return;
    }
    openProjectFiles();
  }

  function handleMobileProjectFilesBack() {
    if (mobileProjectFilesView === "editor") {
      setMobileProjectFilesView("tree");
      return;
    }
    setProjectFilesOpen(false);
    setMobileProjectFilesView("tree");
  }

  function selectMobileProject(projectID: string) {
    setMobileNavOpen(false);
    onSelectProject(projectID);
  }

  function selectSidebarChannel(channel: Channel) {
    setMainView("chat");
    onSelectChannel(channel);
  }

  function selectMobileChannel(channel: Channel) {
    setMobileNavOpen(false);
    selectSidebarChannel(channel);
  }

  async function submitChannel() {
    const name = channelName.trim();
    if (!name) return;
    const teamMaxBatches = Number.parseInt(channelTeamMaxBatches, 10);
    const teamMaxRuns = Number.parseInt(channelTeamMaxRuns, 10);
    const created = await onCreateChannel(name, channelType, {
      team_max_batches: teamMaxBatches,
      team_max_runs: teamMaxRuns,
    });
    setChannelName("");
    setChannelType("text");
    setChannelTeamMaxBatches("6");
    setChannelTeamMaxRuns("12");
    setChannelDraftOpen(false);
    onSelectChannel(created);
  }

  async function submitAgent() {
    const name = newAgentName.trim();
    if (!name) return;
    const handle = normalizeAgentHandle(newAgentHandle || name);
    if (visibleAgents.some((agent) => agent.handle === handle)) {
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
  const headerTitle = mainView === "metrics" ? "Metrics" : title;
  const headerSubtitle = mainView === "metrics" ? project?.name ?? "No project" : subtitle;
  const activityLabel = conversationActivityLabel(connectionStatus, streaming.length > 0);
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
          {projectFilesOpen ? (
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11"
              title={mobileProjectFilesView === "editor" ? "Back to files" : "Back to chat"}
              aria-label={mobileProjectFilesView === "editor" ? "Back to files" : "Back to chat"}
              onClick={handleMobileProjectFilesBack}
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
          ) : null}
          {!projectFilesOpen ? (
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
          ) : null}
          {!projectFilesOpen && selectedChannel?.type === "thread" && activeThread ? (
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
            <h1 className="truncate text-sm font-semibold">{projectFilesOpen ? "Project files" : headerTitle}</h1>
            <p className="truncate text-xs text-muted-foreground">
              {projectFilesOpen ? projectWorkspace?.path ?? "No workspace" : headerSubtitle}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className={cn("h-11 w-11", projectFilesOpen && "bg-accent")}
            title="Project files"
            aria-label="Project files"
            aria-pressed={projectFilesOpen}
            disabled={!projectWorkspace?.id}
            onClick={toggleProjectFiles}
          >
            <FolderOpen className="h-5 w-5" />
          </Button>
          {!projectFilesOpen ? (
            <>
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
            </>
          ) : null}
        </div>

        {!projectFilesOpen && activeThread && (
          <div className="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
            {activeConversation && (
              <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                <Activity className="h-3.5 w-3.5 shrink-0" />
                {activityLabel}
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

        {projectFilesOpen ? (
          mobileProjectFilesView === "tree" ? (
            <WorkspaceFileTreePane
              controller={projectFilesController}
              title="Project files"
              ariaLabel="Project files"
              className="min-h-0 flex-1"
              onFileSelected={() => setMobileProjectFilesView("editor")}
            />
          ) : (
            <WorkspaceFileEditorPane
              controller={projectFilesController}
              theme={theme}
              contentAriaLabel="Project file editor"
              className="min-h-0 flex-1"
            />
          )
        ) : mainView === "metrics" ? (
          <MetricsPanel
            project={project}
            selectedChannel={selectedChannel}
            activeConversation={activeConversation}
          />
        ) : (
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
            preferences={preferences}
            theme={theme}
            composerConversation={composerConversation}
            onSelectThread={onSelectThread}
            onCreateThread={onCreateThread}
            onUpdateThread={onUpdateThread}
            onDeleteThread={onDeleteThread}
            onUpdateMessage={onUpdateMessage}
            onDeleteMessage={onDeleteMessage}
            onLoadOlderMessages={onLoadOlderMessages}
            onMessageSent={onMessageSent}
            workspacePath={projectWorkspace?.path}
            onOpenWorkspacePath={openWorkspacePath}
          />
        )}

        <Dialog open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
          <DialogContent
            showCloseButton={false}
            className="left-0 top-0 min-w-0 !h-svh !w-[100svw] !max-w-[100svw] !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-l-0 p-0 sm:!w-[24rem] sm:!max-w-sm"
          >
            <div className="flex h-full min-h-0 min-w-0 flex-col overflow-x-hidden bg-sidebar">
              <div
                className="flex h-14 min-w-0 shrink-0 items-center justify-between gap-3 border-b border-border px-4"
                data-testid="mobile-nav-header"
              >
                <DialogHeader className="min-w-0 flex-1 gap-0 text-left">
                  <DialogTitle className="truncate">Navigation</DialogTitle>
                  <DialogDescription className="truncate">{project?.name ?? "No project"}</DialogDescription>
                </DialogHeader>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-10 w-10"
                    title="Project settings"
                    aria-label="Project settings"
                    disabled={!project}
                    onClick={() => {
                      setMobileNavOpen(false);
                      openProjectSettings();
                    }}
                  >
                    <Settings className="h-5 w-5" />
                  </Button>
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
                    metricsActive={mainView === "metrics"}
                    onSelect={selectMobileChannel}
                    onOpenMetrics={openMetrics}
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
              onUpdateAgent={onUpdateAgent}
              onDeleteAgent={onDeleteAgent}
              onLoadWorkspaceTree={onLoadWorkspaceTree}
              onReadWorkspaceFile={onReadWorkspaceFile}
              onWriteWorkspaceFile={onWriteWorkspaceFile}
              onDeleteWorkspaceFile={onDeleteWorkspaceFile}
              onCreateAgentModal={() => setAgentDraftOpen(true)}
              onClose={() => setMobileAgentPanelOpen(false)}
              theme={theme}
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
          {projectFilesOpen ? (
            <WorkspaceFileTreePane
              controller={projectFilesController}
              title="Project files"
              ariaLabel="Project files"
            />
          ) : (
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
                  metricsActive={mainView === "metrics"}
                  onSelect={selectSidebarChannel}
                  onOpenMetrics={openMetrics}
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
          )}
        </ResizablePanel>

        <ResizableHandle withHandle />

        {/* Message Area */}
        <ResizablePanel defaultSize={projectFilesOpen ? 82 : agentPanelOpen ? 37 : membersPanelOpen ? 62 : 82}>
          {projectFilesOpen ? (
            <WorkspaceFileEditorPane
              controller={projectFilesController}
              theme={theme}
              contentAriaLabel="Project file editor"
              toolbarEnd={
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 shrink-0 bg-accent"
                  title="Close project files"
                  aria-label="Project files"
                  aria-pressed="true"
                  onClick={toggleProjectFiles}
                >
                  <FolderOpen className="h-4 w-4" />
                </Button>
              }
            />
          ) : (
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
                    {activityLabel}
                  </span>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-8 w-8", mainView === "metrics" && "bg-accent")}
                  title="Metrics"
                  aria-label="Metrics"
                  aria-pressed={mainView === "metrics"}
                  disabled={!project && !selectedChannel}
                  onClick={openMetrics}
                >
                  <BarChart3 className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  title="Project files"
                  aria-label="Project files"
                  aria-pressed="false"
                  disabled={!projectWorkspace?.id}
                  onClick={toggleProjectFiles}
                >
                  <FolderOpen className="h-4 w-4" />
                </Button>
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

            {mainView === "metrics" ? (
              <MetricsPanel
                project={project}
                selectedChannel={selectedChannel}
                activeConversation={activeConversation}
              />
            ) : (
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
              preferences={preferences}
              theme={theme}
              composerConversation={composerConversation}
              onSelectThread={onSelectThread}
              onCreateThread={onCreateThread}
              onUpdateThread={onUpdateThread}
              onDeleteThread={onDeleteThread}
              onUpdateMessage={onUpdateMessage}
              onDeleteMessage={onDeleteMessage}
              onLoadOlderMessages={onLoadOlderMessages}
              onMessageSent={onMessageSent}
              workspacePath={projectWorkspace?.path}
              onOpenWorkspacePath={openWorkspacePath}
            />
            )}
          </div>
          )}
        </ResizablePanel>

        {/* Members Panel */}
        {membersPanelOpen && !agentPanelOpen && !projectFilesOpen && (
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
        {agentPanelOpen && !projectFilesOpen && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel defaultSize={45} minSize={32} maxSize={60}>
              <AgentDetailsPanel
                selectedChannel={selectedChannel}
                projectWorkspace={projectWorkspace}
                agents={activeAgents}
                boundAgents={boundAgents}
                selectedAgent={selectedAgent}
                onUpdateAgent={onUpdateAgent}
                onDeleteAgent={onDeleteAgent}
                onLoadWorkspaceTree={onLoadWorkspaceTree}
                onReadWorkspaceFile={onReadWorkspaceFile}
                onWriteWorkspaceFile={onWriteWorkspaceFile}
                onDeleteWorkspaceFile={onDeleteWorkspaceFile}
                onCreateAgentModal={() => setAgentDraftOpen(true)}
                onClose={() => setAgentPanelOpen(false)}
                theme={theme}
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
        <DialogContent className="max-h-[calc(100vh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>User settings</DialogTitle>
            <DialogDescription>Session and workspace details.</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 space-y-4 overflow-y-auto py-2 pr-1" data-testid="user-settings-scroll">
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
              <div>
                <h3 className="text-sm font-medium">Message metrics</h3>
                <p className="text-xs text-muted-foreground">
                  {preferencesPending ? "Saving" : "Shown under bot replies"}
                </p>
              </div>
              <label className="flex items-center justify-between gap-4 text-sm">
                <span>Show TTFT</span>
                <Switch
                  checked={showTTFT}
                  disabled={preferencesLoading || preferencesPending}
                  aria-label="Show TTFT"
                  onCheckedChange={(checked) =>
                    void saveMetricDisplayPreferences({ show_ttft: checked, show_tps: showTPS })
                  }
                />
              </label>
              <label className="flex items-center justify-between gap-4 text-sm">
                <span>Show TPS</span>
                <Switch
                  checked={showTPS}
                  disabled={preferencesLoading || preferencesPending}
                  aria-label="Show TPS"
                  onCheckedChange={(checked) =>
                    void saveMetricDisplayPreferences({ show_ttft: showTTFT, show_tps: checked })
                  }
                />
              </label>
              {preferencesError && <p className="text-sm text-destructive">{preferencesError}</p>}
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
            <div className="grid gap-3 rounded-md border border-border p-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h3 className="flex items-center gap-2 text-sm font-medium">
                    <ServerIcon className="h-4 w-4" />
                    Server / SSL
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {serverSettings?.restart_required
                      ? "Saved changes require restart"
                      : `HTTP ${serverSettings?.effective_http_addr ?? serverSettings?.effective_addr ?? "from config"}${serverSettings?.effective_https_addr ? `, HTTPS ${serverSettings.effective_https_addr}` : ""}`}
                  </p>
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <Switch
                    checked={serverTLSEnabled}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    aria-label="Enable HTTPS"
                    onCheckedChange={setServerTLSEnabled}
                  />
                  HTTPS
                </label>
              </div>
              {serverSettings?.addr_override_active && (
                <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
                  AGENTX_ADDR is active ({serverSettings.addr_override_value}); HTTP listen IP and port values are saved to config.toml but the environment override controls HTTP startup.
                </p>
              )}
              {serverSettingsError && (
                <p className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {serverSettingsError}
                </p>
              )}
              <div className="grid gap-3 sm:grid-cols-[1fr_8rem_8rem]">
                <div className="space-y-2">
                  <Label htmlFor="server-listen-ip">Listen IP</Label>
                  <Input
                    id="server-listen-ip"
                    value={serverListenIP}
                    onChange={(event) => setServerListenIP(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="server-listen-port">HTTP port</Label>
                  <Input
                    id="server-listen-port"
                    value={serverListenPort}
                    onChange={(event) => setServerListenPort(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    inputMode="numeric"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="server-tls-listen-port">HTTPS port</Label>
                  <Input
                    id="server-tls-listen-port"
                    value={serverTLSListenPort}
                    onChange={(event) => setServerTLSListenPort(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    inputMode="numeric"
                  />
                </div>
              </div>
              {!serverListenPortValid && (
                <p className="text-xs text-destructive">HTTP port must be between 1 and 65535.</p>
              )}
              {!serverTLSListenPortValid && (
                <p className="text-xs text-destructive">HTTPS port must be between 1 and 65535.</p>
              )}
              {serverPortsConflict && (
                <p className="text-xs text-destructive">HTTP and HTTPS ports must be different.</p>
              )}
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="server-cert-file">Certificate path</Label>
                  <Input
                    id="server-cert-file"
                    value={serverTLSCertFile}
                    onChange={(event) => setServerTLSCertFile(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    placeholder="/etc/agentx/cert.pem"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="server-key-file">Private key path</Label>
                  <Input
                    id="server-key-file"
                    value={serverTLSKeyFile}
                    onChange={(event) => setServerTLSKeyFile(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    placeholder="/etc/agentx/key.pem"
                  />
                </div>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="server-cert-pem">Certificate PEM</Label>
                  <Input
                    type="file"
                    accept=".pem,.crt,.cert,text/plain"
                    disabled={serverSettingsLoading || serverSettingsPending}
                    onChange={(event) => {
                      const input = event.currentTarget;
                      const file = input.files?.[0];
                      if (!file) return;
                      void readServerPEMUpload(file, setServerTLSCertPEM).finally(() => {
                        input.value = "";
                      });
                    }}
                  />
                  <Textarea
                    id="server-cert-pem"
                    value={serverTLSCertPEM}
                    onChange={(event) => setServerTLSCertPEM(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    placeholder="Paste certificate PEM"
                    className="h-32 min-h-32 max-h-48 resize-y overflow-auto [field-sizing:fixed] font-mono text-xs"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="server-key-pem">Private key PEM</Label>
                  <Input
                    type="file"
                    accept=".pem,.key,text/plain"
                    disabled={serverSettingsLoading || serverSettingsPending}
                    onChange={(event) => {
                      const input = event.currentTarget;
                      const file = input.files?.[0];
                      if (!file) return;
                      void readServerPEMUpload(file, setServerTLSKeyPEM).finally(() => {
                        input.value = "";
                      });
                    }}
                  />
                  <Textarea
                    id="server-key-pem"
                    value={serverTLSKeyPEM}
                    onChange={(event) => setServerTLSKeyPEM(event.target.value)}
                    disabled={serverSettingsLoading || serverSettingsPending}
                    placeholder="Paste private key PEM"
                    className="h-32 min-h-32 max-h-48 resize-y overflow-auto [field-sizing:fixed] font-mono text-xs"
                  />
                </div>
              </div>
              {serverTLSEnabled && (!serverTLSCertAvailable || !serverTLSKeyAvailable) && (
                <p className="text-xs text-destructive">
                  HTTPS requires a certificate and private key path or pasted PEM content.
                </p>
              )}
              {(serverSettingsActionError || serverSettingsActionStatus) && (
                <p className={cn("text-sm", serverSettingsActionError ? "text-destructive" : "text-muted-foreground")}>
                  {serverSettingsActionError ?? serverSettingsActionStatus}
                </p>
              )}
              <div className="flex justify-end">
                <Button
                  type="button"
                  onClick={saveServerSettings}
                  disabled={serverSettingsSaveDisabled}
                >
                  {serverTLSCertPEM || serverTLSKeyPEM ? <Upload className="h-4 w-4" /> : <Save className="h-4 w-4" />}
                  Save Server
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
            <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
              <div className="space-y-1">
                <p className="text-sm font-medium">Team discussion budget</p>
                <p className="text-xs leading-5 text-muted-foreground">
                  Used when a message mentions agents. The first mentioned agent leads each round, and agent runs cap total sequential replies before the final answer.
                </p>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="channel-team-batches">Discussion rounds</Label>
                  <Input
                    id="channel-team-batches"
                    type="number"
                    min={1}
                    max={20}
                    value={channelTeamMaxBatches}
                    onChange={(e) => setChannelTeamMaxBatches(e.target.value)}
                    aria-label="Team discussion rounds"
                    title="Maximum leader-led discussion rounds before the final answer."
                  />
                  <p className="text-[11px] leading-4 text-muted-foreground">1-20 handoff rounds.</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="channel-team-runs">Agent run budget</Label>
                  <Input
                    id="channel-team-runs"
                    type="number"
                    min={1}
                    max={50}
                    value={channelTeamMaxRuns}
                    onChange={(e) => setChannelTeamMaxRuns(e.target.value)}
                    aria-label="Team agent run budget"
                    title="Maximum sequential agent replies across the leader-led discussion."
                  />
                  <p className="text-[11px] leading-4 text-muted-foreground">1-50 discussion replies.</p>
                </div>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChannelDraftOpen(false)}>Cancel</Button>
            <Button
              onClick={submitChannel}
              disabled={!channelName.trim() || !channelTeamMaxBatches.trim() || !channelTeamMaxRuns.trim()}
            >
              Create
            </Button>
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
                <Input
                  id="new-agent-effort"
                  value={newAgentEffort}
                  onChange={(e) => setNewAgentEffort(e.target.value)}
                  list="new-agent-effort-suggestions"
                  placeholder="default or custom"
                  aria-label="New agent effort"
                  autoComplete="off"
                />
                <datalist id="new-agent-effort-suggestions">
                  {AGENT_EFFORT_OPTIONS.map((option) => (
                    <option key={option} value={option} />
                  ))}
                </datalist>
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
