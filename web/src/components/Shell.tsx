import {
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { Channel, UserPreferences } from "../api/types";
import {
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
import { ShellDialogs } from "./shell/ShellDialogs";
import { MobileShell } from "./shell/MobileShell";
import { DesktopShell } from "./shell/DesktopShell";
import { useShellProjectFiles } from "./shell/useShellProjectFiles";
import type { ShellProps } from "./shell/types";
import {
  blurActiveElement,
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
  pendingQuestion,
  queuedPrompts,
  connectionStatus,
  notificationSettings,
  notificationSettingsLoading,
  serverSettings,
  serverSettingsLoading,
  serverSettingsError,
  toolUpdates,
  toolUpdatesLoading,
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
  onUpdateToolUpdateSettings,
  onCheckToolUpdates,
  onRunToolUpdate,
  onUpdateUserPreferences,
  onTestNotificationSettings,
  onLoadWorkspaceTree,
  onSearchWorkspace,
  onReadWorkspaceFile,
  onFetchWorkspaceFileBlob,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onCreateWorkspaceEntry,
  onMoveWorkspaceEntry,
  onDeleteWorkspaceEntry,
  onLoadWorkspaceGitStatus,
  onLoadWorkspaceGitHistory,
  onLoadWorkspaceGitDiff,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onRespondToQuestion,
  onSteerQueuedPrompt,
  onDeleteQueuedPrompt,
  onMessageSent,
  onToggleTheme,
  onLogout
}: ShellProps) {
  const [agentPanelOpen, setAgentPanelOpen] = useState(false);
  const [focusedAgentID, setFocusedAgentID] = useState("");
  const [membersPanelOpen, setMembersPanelOpen] = useState(false);
  const [terminalOpen, setTerminalOpen] = useState(false);
  const [terminalHeightPct, setTerminalHeightPct] = useState(38);
  const [mainView, setMainView] = useState<"chat" | "metrics" | "tasks">("chat");
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [mobileAgentPanelOpen, setMobileAgentPanelOpen] = useState(false);
  const [mobileMembersPanelOpen, setMobileMembersPanelOpen] = useState(false);
  const [isMobileLayout, setIsMobileLayout] = useState(() =>
    typeof window !== "undefined"
      ? window.matchMedia("(max-width: 767px)").matches
      : false
  );
  const terminalResizeContainerRef = useRef<HTMLDivElement | null>(null);
  const [projectName, setProjectName] = useState("");
  const [projectWorkspacePath, setProjectWorkspacePath] = useState("");
  const [projectCreateError, setProjectCreateError] = useState<string | null>(null);
  const [projectCreatePending, setProjectCreatePending] = useState(false);
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
  const [hideAvatars, setHideAvatars] = useState(preferences.hide_avatars);
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
  const [toolAutoEnabled, setToolAutoEnabled] = useState(false);
  const [toolTimeOfDay, setToolTimeOfDay] = useState("04:00");
  const [toolTimezone, setToolTimezone] = useState("Local");
  const [toolClaudeEnabled, setToolClaudeEnabled] = useState(true);
  const [toolCodexEnabled, setToolCodexEnabled] = useState(true);
  const [toolSettingsPending, setToolSettingsPending] = useState(false);
  const [toolActionStatus, setToolActionStatus] = useState<string | null>(null);
  const [toolActionError, setToolActionError] = useState<string | null>(null);
  const [preferencesPending, setPreferencesPending] = useState(false);
  const [preferencesError, setPreferencesError] = useState<string | null>(null);
  const {
    projectFilesOpen,
    projectFileTreeCollapsed,
    setProjectFileTreeCollapsed,
    mobileProjectFilesView,
    setMobileProjectFilesView,
    mobileEditorHeaderCollapsed,
    setMobileEditorHeaderCollapsed,
    projectFilesController,
    openWorkspacePath,
    toggleProjectFiles,
    closeProjectFiles,
    handleMobileProjectFilesBack,
  } = useShellProjectFiles({
    projectID: project?.id,
    projectWorkspace,
    setMobileNavOpen,
    setMobileAgentPanelOpen,
    setMobileMembersPanelOpen,
    onLoadWorkspaceTree,
    onSearchWorkspace,
    onReadWorkspaceFile,
    onFetchWorkspaceFileBlob,
    onWriteWorkspaceFile,
    onDeleteWorkspaceFile,
    onCreateWorkspaceEntry,
    onMoveWorkspaceEntry,
    onDeleteWorkspaceEntry,
    onLoadWorkspaceGitStatus,
    onLoadWorkspaceGitHistory,
    onLoadWorkspaceGitDiff,
  });
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
  const terminalAllowed = organization?.role === "owner" || organization?.role === "admin";

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
    if (!terminalAllowed || !projectWorkspace?.id) {
      setTerminalOpen(false);
    }
  }, [projectWorkspace?.id, terminalAllowed]);

  const startTerminalResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const container = terminalResizeContainerRef.current;
    if (!container) return;
    event.preventDefault();
    const rect = container.getBoundingClientRect();
    if (rect.height <= 0) return;
    const updateHeight = (clientY: number) => {
      const next = ((rect.bottom - clientY) / rect.height) * 100;
      setTerminalHeightPct(Math.min(70, Math.max(22, next)));
    };
    updateHeight(event.clientY);
    const handlePointerMove = (moveEvent: PointerEvent) => updateHeight(moveEvent.clientY);
    const handlePointerUp = () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
    };
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("pointercancel", handlePointerUp);
  }, []);

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
    preferences.show_tps,
    preferences.hide_avatars
  ]);

  async function submitProject() {
    const name = projectName.trim();
    if (!name) return;
    const workspacePath = projectWorkspacePath.trim();
    setProjectCreateError(null);
    setProjectCreatePending(true);
    try {
      const created = await onCreateProject({
        name,
        ...(workspacePath ? { workspace_path: workspacePath } : {}),
      });
      setProjectName("");
      setProjectWorkspacePath("");
      setProjectDraftOpen(false);
      onSelectProject(created.id);
    } catch (err) {
      setProjectCreateError(err instanceof Error ? err.message : "Create project failed");
    } finally {
      setProjectCreatePending(false);
    }
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
    setProjectCreateError(null);
    setProjectDraftOpen(true);
  }

  function openAccountSettings() {
    blurActiveElement();
    setBrowserPermission(browserNotificationPermission());
    syncNotificationDraft();
    syncServerSettingsDraft();
    syncToolUpdateDraft();
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

  function syncToolUpdateDraft() {
    const settings = toolUpdates?.settings;
    setToolAutoEnabled(settings?.auto_enabled ?? false);
    setToolTimeOfDay(settings?.time_of_day ?? "04:00");
    setToolTimezone(settings?.timezone ?? "Local");
    setToolClaudeEnabled(settings?.claude_enabled ?? true);
    setToolCodexEnabled(settings?.codex_enabled ?? true);
    setToolActionError(null);
    setToolActionStatus(null);
  }

  function syncPreferenceDraft() {
    setShowTTFT(preferences.show_ttft);
    setShowTPS(preferences.show_tps);
    setHideAvatars(preferences.hide_avatars);
    setPreferencesError(null);
  }

  async function saveUserPreferences(next: UserPreferences) {
    setShowTTFT(next.show_ttft);
    setShowTPS(next.show_tps);
    setHideAvatars(next.hide_avatars);
    setPreferencesError(null);
    setPreferencesPending(true);
    try {
      await onUpdateUserPreferences(next);
    } catch (err) {
      setShowTTFT(preferences.show_ttft);
      setShowTPS(preferences.show_tps);
      setHideAvatars(preferences.hide_avatars);
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

  async function saveToolUpdateSettings() {
    setToolActionError(null);
    setToolActionStatus(null);
    setToolSettingsPending(true);
    try {
      await onUpdateToolUpdateSettings({
        auto_enabled: toolAutoEnabled,
        time_of_day: toolTimeOfDay,
        timezone: toolTimezone.trim() || "Local",
        claude_enabled: toolClaudeEnabled,
        codex_enabled: toolCodexEnabled,
      });
      setToolActionStatus("Saved");
    } catch (err) {
      setToolActionError(err instanceof Error ? err.message : "Save runtime update settings failed");
    } finally {
      setToolSettingsPending(false);
    }
  }

  async function checkAllToolUpdates() {
    setToolActionError(null);
    setToolActionStatus("Checking");
    try {
      await onCheckToolUpdates("all");
      setToolActionStatus("Checked");
    } catch (err) {
      setToolActionError(err instanceof Error ? err.message : "Check failed");
      setToolActionStatus(null);
    }
  }

  async function runAllToolUpdates() {
    setToolActionError(null);
    setToolActionStatus("Updating");
    try {
      await onRunToolUpdate("all");
      setToolActionStatus("Update started");
    } catch (err) {
      setToolActionError(err instanceof Error ? err.message : "Update failed");
      setToolActionStatus(null);
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

  function openMetrics() {
    if (!project && !selectedChannel) return;
    blurActiveElement();
    closeProjectFiles();
    setAgentPanelOpen(false);
    setMembersPanelOpen(false);
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setMainView("metrics");
  }

  function openTasks() {
    if (!project) return;
    blurActiveElement();
    closeProjectFiles();
    setAgentPanelOpen(false);
    setMembersPanelOpen(false);
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
    setMainView("tasks");
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
  const headerTitle = mainView === "metrics" ? "Metrics" : mainView === "tasks" ? "Tasks" : title;
  const headerSubtitle =
    mainView === "metrics" || mainView === "tasks" ? project?.name ?? "No project" : subtitle;
  const activityLabel = conversationActivityLabel(connectionStatus, streaming.length > 0);
  const composerConversation =
    activeConversation && selectedChannel?.type === "text"
      ? { type: activeConversation.type, id: activeConversation.id, label: `#${selectedChannel.name}` }
      : activeConversation && activeThread
        ? { type: activeConversation.type, id: activeConversation.id, label: activeThread.title }
        : undefined;
  const showMobileProjectFilesButton = !(projectFilesOpen && mobileProjectFilesView === "editor");
  const showMobileEditorHeaderControls = projectFilesOpen && mobileProjectFilesView === "editor";
  const mobileTerminalOpen = terminalOpen && !projectFilesOpen;

  return (
    <div className="flex h-dvh w-screen overflow-hidden select-none" data-testid="agentx-shell">
      {isMobileLayout ? (
        <MobileShell
          projectFilesOpen={projectFilesOpen}
          mobileProjectFilesView={mobileProjectFilesView}
          handleMobileProjectFilesBack={handleMobileProjectFilesBack}
          mobileTerminalOpen={mobileTerminalOpen}
          setTerminalOpen={setTerminalOpen}
          selectedChannel={selectedChannel}
          activeThread={activeThread}
          onSelectChannel={onSelectChannel}
          headerTitle={headerTitle}
          projectWorkspace={projectWorkspace}
          headerSubtitle={headerSubtitle}
          showMobileEditorHeaderControls={showMobileEditorHeaderControls}
          toggleProjectFiles={toggleProjectFiles}
          mobileEditorHeaderCollapsed={mobileEditorHeaderCollapsed}
          setMobileEditorHeaderCollapsed={setMobileEditorHeaderCollapsed}
          showMobileProjectFilesButton={showMobileProjectFilesButton}
          mainView={mainView}
          project={project}
          openTasks={openTasks}
          terminalAllowed={terminalAllowed}
          terminalOpen={terminalOpen}
          setMobileMembersPanelOpen={setMobileMembersPanelOpen}
          setMobileAgentPanelOpen={setMobileAgentPanelOpen}
          activityLabel={activityLabel}
          threadActionPending={threadActionPending}
          deleteActiveThread={deleteActiveThread}
          setThreadTitleDraft={setThreadTitleDraft}
          setThreadActionError={setThreadActionError}
          setThreadEditOpen={setThreadEditOpen}
          theme={theme}
          projectFilesController={projectFilesController}
          setMobileProjectFilesView={setMobileProjectFilesView}
          channels={channels}
          threads={threads}
          activeConversation={activeConversation}
          activeAgents={activeAgents}
          messages={messages}
          messagesLoading={messagesLoading}
          olderMessagesLoading={olderMessagesLoading}
          hasOlderMessages={hasOlderMessages}
          streaming={streaming}
          pendingQuestion={pendingQuestion}
          queuedPrompts={queuedPrompts}
          boundAgents={boundAgents}
          preferences={preferences}
          composerConversation={composerConversation}
          onSelectThread={onSelectThread}
          onCreateThread={onCreateThread}
          onUpdateThread={onUpdateThread}
          onDeleteThread={onDeleteThread}
          onUpdateMessage={onUpdateMessage}
          onDeleteMessage={onDeleteMessage}
          onLoadOlderMessages={onLoadOlderMessages}
          onRespondToQuestion={onRespondToQuestion}
          onSteerQueuedPrompt={onSteerQueuedPrompt}
          onDeleteQueuedPrompt={onDeleteQueuedPrompt}
          onMessageSent={onMessageSent}
          openWorkspacePath={openWorkspacePath}
          mobileNavOpen={mobileNavOpen}
          setMobileNavOpen={setMobileNavOpen}
          projects={projects}
          selectMobileProject={selectMobileProject}
          openCreateProject={openCreateProject}
          openProjectSettings={openProjectSettings}
          openMetrics={openMetrics}
          onUpdateChannel={onUpdateChannel}
          onDeleteChannel={onDeleteChannel}
          setChannelDraftOpen={setChannelDraftOpen}
          contextLoading={contextLoading}
          setFocusedAgentID={setFocusedAgentID}
          setAgentDraftOpen={setAgentDraftOpen}
          user={user}
          organization={organization}
          openAccountSettings={openAccountSettings}
          onToggleTheme={onToggleTheme}
          onLogout={onLogout}
          mobileMembersPanelOpen={mobileMembersPanelOpen}
          onSaveChannelAgents={onSaveChannelAgents}
          mobileAgentPanelOpen={mobileAgentPanelOpen}
          selectedAgent={selectedAgent}
          onUpdateAgent={onUpdateAgent}
          onDeleteAgent={onDeleteAgent}
          toolUpdates={toolUpdates}
          toolUpdatesLoading={toolUpdatesLoading}
          onCheckToolUpdates={onCheckToolUpdates}
          onRunToolUpdate={onRunToolUpdate}
          onLoadWorkspaceTree={onLoadWorkspaceTree}
          onSearchWorkspace={onSearchWorkspace}
          onReadWorkspaceFile={onReadWorkspaceFile}
          onFetchWorkspaceFileBlob={onFetchWorkspaceFileBlob}
          onWriteWorkspaceFile={onWriteWorkspaceFile}
          onDeleteWorkspaceFile={onDeleteWorkspaceFile}
          onCreateWorkspaceEntry={onCreateWorkspaceEntry}
          onMoveWorkspaceEntry={onMoveWorkspaceEntry}
          onDeleteWorkspaceEntry={onDeleteWorkspaceEntry}
          selectMobileChannel={selectMobileChannel}
        />
      ) : null}

      {!isMobileLayout ? (
        <DesktopShell
          projectFilesOpen={projectFilesOpen}
          projectDraftOpen={projectDraftOpen}
          openCreateProject={openCreateProject}
          projects={projects}
          project={project}
          theme={theme}
          onToggleTheme={onToggleTheme}
          onLogout={onLogout}
          onSelectProject={onSelectProject}
          openProjectSettings={openProjectSettings}
          mainView={mainView}
          channels={channels}
          selectedChannel={selectedChannel}
          selectSidebarChannel={selectSidebarChannel}
          openMetrics={openMetrics}
          onUpdateChannel={onUpdateChannel}
          onDeleteChannel={onDeleteChannel}
          setChannelDraftOpen={setChannelDraftOpen}
          openTasks={openTasks}
          activeAgents={activeAgents}
          boundAgents={boundAgents}
          contextLoading={contextLoading}
          setFocusedAgentID={setFocusedAgentID}
          setMembersPanelOpen={setMembersPanelOpen}
          setAgentPanelOpen={setAgentPanelOpen}
          setAgentDraftOpen={setAgentDraftOpen}
          onSaveChannelAgents={onSaveChannelAgents}
          user={user}
          organization={organization}
          openAccountSettings={openAccountSettings}
          agentPanelOpen={agentPanelOpen}
          membersPanelOpen={membersPanelOpen}
          activeThread={activeThread}
          title={title}
          subtitle={subtitle}
          onSelectChannel={onSelectChannel}
          threadActionPending={threadActionPending}
          deleteActiveThread={deleteActiveThread}
          setThreadTitleDraft={setThreadTitleDraft}
          setThreadActionError={setThreadActionError}
          setThreadEditOpen={setThreadEditOpen}
          activeConversation={activeConversation}
          activityLabel={activityLabel}
          projectWorkspace={projectWorkspace}
          toggleProjectFiles={toggleProjectFiles}
          terminalAllowed={terminalAllowed}
          terminalOpen={terminalOpen}
          setTerminalOpen={setTerminalOpen}
          terminalResizeContainerRef={terminalResizeContainerRef}
          startTerminalResize={startTerminalResize}
          terminalHeightPct={terminalHeightPct}
          threads={threads}
          messages={messages}
          messagesLoading={messagesLoading}
          olderMessagesLoading={olderMessagesLoading}
          hasOlderMessages={hasOlderMessages}
          streaming={streaming}
          pendingQuestion={pendingQuestion}
          queuedPrompts={queuedPrompts}
          preferences={preferences}
          composerConversation={composerConversation}
          onSelectThread={onSelectThread}
          onCreateThread={onCreateThread}
          onUpdateThread={onUpdateThread}
          onDeleteThread={onDeleteThread}
          onUpdateMessage={onUpdateMessage}
          onDeleteMessage={onDeleteMessage}
          onLoadOlderMessages={onLoadOlderMessages}
          onRespondToQuestion={onRespondToQuestion}
          onSteerQueuedPrompt={onSteerQueuedPrompt}
          onDeleteQueuedPrompt={onDeleteQueuedPrompt}
          onMessageSent={onMessageSent}
          openWorkspacePath={openWorkspacePath}
          selectedAgent={selectedAgent}
          onUpdateAgent={onUpdateAgent}
          onDeleteAgent={onDeleteAgent}
          toolUpdates={toolUpdates}
          toolUpdatesLoading={toolUpdatesLoading}
          onCheckToolUpdates={onCheckToolUpdates}
          onRunToolUpdate={onRunToolUpdate}
          onLoadWorkspaceTree={onLoadWorkspaceTree}
          onSearchWorkspace={onSearchWorkspace}
          onReadWorkspaceFile={onReadWorkspaceFile}
          onFetchWorkspaceFileBlob={onFetchWorkspaceFileBlob}
          onWriteWorkspaceFile={onWriteWorkspaceFile}
          onDeleteWorkspaceFile={onDeleteWorkspaceFile}
          onCreateWorkspaceEntry={onCreateWorkspaceEntry}
          onMoveWorkspaceEntry={onMoveWorkspaceEntry}
          onDeleteWorkspaceEntry={onDeleteWorkspaceEntry}
          projectFilesController={projectFilesController}
          projectFileTreeCollapsed={projectFileTreeCollapsed}
          setProjectFileTreeCollapsed={setProjectFileTreeCollapsed}
          setMobileProjectFilesView={setMobileProjectFilesView}
        />
      ) : null}

      <ShellDialogs
        user={user}
        organization={organization}
        project={project}
        projectWorkspace={projectWorkspace}
        selectedChannel={selectedChannel}
        notificationSettings={notificationSettings}
        notificationSettingsLoading={notificationSettingsLoading}
        serverSettings={serverSettings}
        serverSettingsLoading={serverSettingsLoading}
        serverSettingsError={serverSettingsError}
        toolUpdates={toolUpdates}
        toolUpdatesLoading={toolUpdatesLoading}
        toolAutoEnabled={toolAutoEnabled}
        setToolAutoEnabled={setToolAutoEnabled}
        toolTimeOfDay={toolTimeOfDay}
        setToolTimeOfDay={setToolTimeOfDay}
        toolTimezone={toolTimezone}
        setToolTimezone={setToolTimezone}
        toolClaudeEnabled={toolClaudeEnabled}
        setToolClaudeEnabled={setToolClaudeEnabled}
        toolCodexEnabled={toolCodexEnabled}
        setToolCodexEnabled={setToolCodexEnabled}
        toolSettingsPending={toolSettingsPending}
        toolActionStatus={toolActionStatus}
        toolActionError={toolActionError}
        saveToolUpdateSettings={saveToolUpdateSettings}
        checkAllToolUpdates={checkAllToolUpdates}
        runAllToolUpdates={runAllToolUpdates}
        preferences={preferences}
        preferencesLoading={preferencesLoading}
        onLogout={onLogout}
        threadEditOpen={threadEditOpen}
        setThreadEditOpen={setThreadEditOpen}
        threadTitleDraft={threadTitleDraft}
        setThreadTitleDraft={setThreadTitleDraft}
        threadActionError={threadActionError}
        threadActionPending={threadActionPending}
        submitActiveThreadTitle={submitActiveThreadTitle}
        accountSettingsOpen={accountSettingsOpen}
        setAccountSettingsOpen={setAccountSettingsOpen}
        browserPermission={browserPermission}
        enableBrowserNotifications={enableBrowserNotifications}
        preferencesPending={preferencesPending}
        preferencesError={preferencesError}
        showTTFT={showTTFT}
        showTPS={showTPS}
        hideAvatars={hideAvatars}
        saveUserPreferences={saveUserPreferences}
        webhookEnabled={webhookEnabled}
        setWebhookEnabled={setWebhookEnabled}
        webhookURL={webhookURL}
        setWebhookURL={setWebhookURL}
        webhookSecret={webhookSecret}
        setWebhookSecret={setWebhookSecret}
        notificationActionError={notificationActionError}
        notificationActionStatus={notificationActionStatus}
        notificationTestPending={notificationTestPending}
        notificationSavePending={notificationSavePending}
        sendTestWebhook={sendTestWebhook}
        saveNotificationSettings={saveNotificationSettings}
        serverListenIP={serverListenIP}
        setServerListenIP={setServerListenIP}
        serverListenPort={serverListenPort}
        setServerListenPort={setServerListenPort}
        serverTLSEnabled={serverTLSEnabled}
        setServerTLSEnabled={setServerTLSEnabled}
        serverTLSListenPort={serverTLSListenPort}
        setServerTLSListenPort={setServerTLSListenPort}
        serverTLSCertFile={serverTLSCertFile}
        setServerTLSCertFile={setServerTLSCertFile}
        serverTLSKeyFile={serverTLSKeyFile}
        setServerTLSKeyFile={setServerTLSKeyFile}
        serverTLSCertPEM={serverTLSCertPEM}
        setServerTLSCertPEM={setServerTLSCertPEM}
        serverTLSKeyPEM={serverTLSKeyPEM}
        setServerTLSKeyPEM={setServerTLSKeyPEM}
        serverListenPortValid={serverListenPortValid}
        serverTLSListenPortValid={serverTLSListenPortValid}
        serverPortsConflict={serverPortsConflict}
        serverTLSCertAvailable={serverTLSCertAvailable}
        serverTLSKeyAvailable={serverTLSKeyAvailable}
        serverSettingsPending={serverSettingsPending}
        serverSettingsActionError={serverSettingsActionError}
        serverSettingsActionStatus={serverSettingsActionStatus}
        serverSettingsSaveDisabled={serverSettingsSaveDisabled}
        readServerPEMUpload={readServerPEMUpload}
        saveServerSettings={saveServerSettings}
        projectEditOpen={projectEditOpen}
        setProjectEditOpen={setProjectEditOpen}
        projectEditName={projectEditName}
        setProjectEditName={setProjectEditName}
        projectEditWorkspacePath={projectEditWorkspacePath}
        setProjectEditWorkspacePath={setProjectEditWorkspacePath}
        projectEditEmoji={projectEditEmoji}
        setProjectEditEmoji={setProjectEditEmoji}
        projectEditColor={projectEditColor}
        setProjectEditColor={setProjectEditColor}
        projectEditError={projectEditError}
        projectEditPending={projectEditPending}
        submitProjectEdit={submitProjectEdit}
        deleteActiveProject={deleteActiveProject}
        projectDraftOpen={projectDraftOpen}
        setProjectDraftOpen={setProjectDraftOpen}
        projectName={projectName}
        setProjectName={setProjectName}
        projectWorkspacePath={projectWorkspacePath}
        setProjectWorkspacePath={setProjectWorkspacePath}
        projectCreateError={projectCreateError}
        setProjectCreateError={setProjectCreateError}
        projectCreatePending={projectCreatePending}
        submitProject={submitProject}
        channelDraftOpen={channelDraftOpen}
        setChannelDraftOpen={setChannelDraftOpen}
        channelName={channelName}
        setChannelName={setChannelName}
        channelType={channelType}
        setChannelType={setChannelType}
        channelTeamMaxBatches={channelTeamMaxBatches}
        setChannelTeamMaxBatches={setChannelTeamMaxBatches}
        channelTeamMaxRuns={channelTeamMaxRuns}
        setChannelTeamMaxRuns={setChannelTeamMaxRuns}
        submitChannel={submitChannel}
        agentDraftOpen={agentDraftOpen}
        setAgentDraftOpen={setAgentDraftOpen}
        newAgentName={newAgentName}
        setNewAgentName={setNewAgentName}
        newAgentDescription={newAgentDescription}
        setNewAgentDescription={setNewAgentDescription}
        newAgentHandle={newAgentHandle}
        setNewAgentHandle={setNewAgentHandle}
        newAgentKind={newAgentKind}
        setNewAgentKind={setNewAgentKind}
        newAgentModel={newAgentModel}
        setNewAgentModel={setNewAgentModel}
        newAgentEffort={newAgentEffort}
        setNewAgentEffort={setNewAgentEffort}
        newAgentFastMode={newAgentFastMode}
        setNewAgentFastMode={setNewAgentFastMode}
        newAgentYoloMode={newAgentYoloMode}
        setNewAgentYoloMode={setNewAgentYoloMode}
        newAgentEmoji={newAgentEmoji}
        setNewAgentEmoji={setNewAgentEmoji}
        newAgentColor={newAgentColor}
        setNewAgentColor={setNewAgentColor}
        newAgentError={newAgentError}
        setNewAgentError={setNewAgentError}
        creatingAgent={creatingAgent}
        submitAgent={submitAgent}
      />
    </div>
  );
}
