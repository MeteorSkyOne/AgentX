import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  agents,
  channelAgents,
  channelThreads,
  checkSelfUpdate,
  checkToolUpdates,
  clearToken,
  conversationContext,
  createAgent,
  createChannel,
  createProject,
  createThread,
  deleteAgent,
  deleteChannel,
  deleteMessage,
  deleteProject,
  deleteThread,
  getToken,
  logout as logoutRequest,
  me,
  notificationSettings,
  organizations,
  projectChannels,
  projects,
  restartServer,
  runSelfUpdate,
  serverSettings,
  setChannelAgents,
  selfUpdate,
  toolUpdates,
  testNotificationSettings,
  updateAgent,
  updateChannel,
  updateMessage,
  updateNotificationSettings,
  updateProject,
  updateServerSettings,
  updateSelfUpdateSettings,
  updateToolUpdateSettings,
  updateThread,
  updateUserPreferences,
  userPreferences,
  workspace,
  runToolUpdate,
  respondToInputRequest,
  retryAgentRun,
  steerQueuedPrompt,
  deleteQueuedPrompt
} from "./api/client";
import type {
  Agent,
  AuthResponse,
  Channel,
  CreateThreadResponse,
  Message,
  NotificationSettings,
  Project,
  SelfUpdateOverview,
  SelfUpdateSettings,
  ServerSettings,
  ServerSettingsUpdatePayload,
  Thread,
  ToolUpdateOverview,
  ToolUpdateSettings,
  ToolUpdateStatus,
  UserPreferences,
} from "./api/types";
import { LoginView } from "./components/LoginView";
import { Shell } from "./components/Shell";
import {
  eventMatchesActiveConversation,
  mergeMessages,
  messageHistoryLoadingForEvent,
  messageMatchesActiveConversation,
  removeMessageAndMarkReferencesDeleted,
  streamingRunHasCompletedMessage
} from "./messages/state";
import type { AgentXEvent } from "./ws/events";
import { useConversationSocket } from "./ws/useConversationSocket";
import { applyTheme, getInitialTheme, storeTheme, type ThemeMode } from "./theme";
import { showAgentInputRequestNotification, showAgentMessageNotification } from "./notifications/browser";
import type { PendingQuestion, QueuedPrompt } from "./components/shell/types";
import type { ActiveConversation, StreamingMessage } from "./app/state";
import {
  activeConversationFromSelection,
  agentNameForID,
  clearPageSelection,
  conversationKey,
  getStoredPageSelection,
  isSystemCommandMessage,
  storePageSelection,
  upsertQueuedPrompt,
} from "./app/state";
import {
  handleCreateWorkspaceEntry,
  handleDeleteWorkspaceEntry,
  handleDeleteWorkspaceFile,
  handleFetchWorkspaceFileBlob,
  handleLoadWorkspaceGitDiff,
  handleLoadWorkspaceGitHistory,
  handleLoadWorkspaceGitStatus,
  handleLoadWorkspaceTree,
  handleMoveWorkspaceEntry,
  handleReadWorkspaceFile,
  handleSearchWorkspace,
  handleWriteWorkspaceFile,
} from "./app/workspaceActions";
import { LoadingSessionView } from "./app/LoadingSessionView";

function toolUpdatesRefetchInterval(query: { state: { data?: ToolUpdateOverview } }): number {
  const hasActiveToolAction = query.state.data?.tools.some(
    (tool) => tool.state === "checking" || tool.state === "updating" || tool.runtime_reset_pending
  );
  return hasActiveToolAction ? 5_000 : 60_000;
}

function selfUpdateRefetchInterval(query: { state: { data?: SelfUpdateOverview } }): number {
  const state = query.state.data?.status.state;
  return state === "checking" || state === "downloading" || state === "replacing" ? 5_000 : 60_000;
}

export default function App() {
  const queryClient = useQueryClient();
  const [initialPageSelection] = useState(() => getStoredPageSelection());
  const [sessionToken, setSessionToken] = useState(() => getToken());
  const [selectedOrganizationID, setSelectedOrganizationID] = useState<string | undefined>(
    initialPageSelection.organizationID
  );
  const [selectedProjectID, setSelectedProjectID] = useState<string | undefined>(
    initialPageSelection.projectID
  );
  const [selectedChannelID, setSelectedChannelID] = useState<string | undefined>(
    initialPageSelection.channelID
  );
  const [activeConversation, setActiveConversation] = useState<ActiveConversation | undefined>(() =>
    activeConversationFromSelection(initialPageSelection)
  );
  const [conversationMessages, setConversationMessages] = useState<Message[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [olderMessagesLoading, setOlderMessagesLoading] = useState(false);
  const [messageHistoryHasMore, setMessageHistoryHasMore] = useState(false);
  const [streamingByRunID, setStreamingByRunID] = useState<Record<string, StreamingMessage>>({});
  const [pendingQuestion, setPendingQuestion] = useState<PendingQuestion | null>(null);
  const [queuedPrompts, setQueuedPrompts] = useState<QueuedPrompt[]>([]);
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme());
  const streamingCacheRef = useRef<Record<string, Record<string, StreamingMessage>>>({});
  const pendingQuestionCacheRef = useRef<Record<string, PendingQuestion | null>>({});
  const queuedPromptsCacheRef = useRef<Record<string, QueuedPrompt[]>>({});
  const hasSession = Boolean(sessionToken);

  useEffect(() => {
    applyTheme(theme);
    storeTheme(theme);
  }, [theme]);

  useEffect(() => {
    if (!hasSession) {
      return;
    }
    storePageSelection({
      organizationID: selectedOrganizationID,
      projectID: selectedProjectID,
      channelID: selectedChannelID,
      conversationType: activeConversation?.type,
      conversationID: activeConversation?.id
    });
  }, [hasSession, selectedOrganizationID, selectedProjectID, selectedChannelID, activeConversation]);

  const meQuery = useQuery({
    queryKey: ["me", sessionToken],
    queryFn: me,
    enabled: hasSession
  });

  const organizationsQuery = useQuery({
    queryKey: ["organizations", sessionToken],
    queryFn: organizations,
    enabled: hasSession && meQuery.isSuccess
  });

  const selectedOrganization = useMemo(
    () => organizationsQuery.data?.find((org) => org.id === selectedOrganizationID),
    [organizationsQuery.data, selectedOrganizationID]
  );

  const projectsQuery = useQuery({
    queryKey: ["projects", selectedOrganizationID],
    queryFn: () => projects(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID)
  });

  const selectedProject = useMemo(
    () => projectsQuery.data?.find((project) => project.id === selectedProjectID),
    [projectsQuery.data, selectedProjectID]
  );

  const selectedProjectWorkspaceQuery = useQuery({
    queryKey: ["workspace", selectedProject?.workspace_id],
    queryFn: () => workspace(selectedProject!.workspace_id),
    enabled: hasSession && Boolean(selectedProject?.workspace_id)
  });

  const channelsQuery = useQuery({
    queryKey: ["project-channels", selectedProjectID],
    queryFn: () => projectChannels(selectedProjectID as string),
    enabled: hasSession && Boolean(selectedProjectID)
  });

  const selectedChannel = useMemo(
    () => channelsQuery.data?.find((channel) => channel.id === selectedChannelID),
    [channelsQuery.data, selectedChannelID]
  );

  const threadsQuery = useQuery({
    queryKey: ["threads", selectedChannelID],
    queryFn: () => channelThreads(selectedChannelID as string),
    enabled: hasSession && selectedChannel?.type === "thread"
  });

  const agentsQuery = useQuery({
    queryKey: ["agents", selectedOrganizationID],
    queryFn: () => agents(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID)
  });

  const notificationSettingsQuery = useQuery({
    queryKey: ["notification-settings", selectedOrganizationID],
    queryFn: () => notificationSettings(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID)
  });

  const serverSettingsQuery = useQuery({
    queryKey: ["server-settings", selectedOrganizationID],
    queryFn: () => serverSettings(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID),
    retry: false
  });

  const toolUpdatesQuery = useQuery({
    queryKey: ["tool-updates", selectedOrganizationID],
    queryFn: () => toolUpdates(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID),
    retry: false,
    refetchInterval: toolUpdatesRefetchInterval,
    refetchIntervalInBackground: false
  });

  const selfUpdateQuery = useQuery({
    queryKey: ["self-update", selectedOrganizationID],
    queryFn: () => selfUpdate(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID),
    retry: false,
    refetchInterval: selfUpdateRefetchInterval,
    refetchIntervalInBackground: false
  });

  const userPreferencesQuery = useQuery({
    queryKey: ["user-preferences", sessionToken],
    queryFn: userPreferences,
    enabled: hasSession && meQuery.isSuccess
  });

  const channelAgentsQuery = useQuery({
    queryKey: ["channel-agents", selectedChannelID],
    queryFn: () => channelAgents(selectedChannelID as string),
    enabled: hasSession && Boolean(selectedChannelID),
    retry: false
  });

  const conversationContextQuery = useQuery({
    queryKey: ["conversation-context", activeConversation?.type, activeConversation?.id],
    queryFn: () => conversationContext(activeConversation!.type, activeConversation!.id),
    enabled: hasSession && Boolean(activeConversation),
    retry: false
  });

  useEffect(() => {
    if (!selectedOrganizationID && organizationsQuery.data && organizationsQuery.data.length > 0) {
      setSelectedOrganizationID(organizationsQuery.data[0].id);
    }
  }, [organizationsQuery.data, selectedOrganizationID]);

  useEffect(() => {
    if (
      selectedOrganizationID &&
      organizationsQuery.data &&
      organizationsQuery.data.length > 0 &&
      !organizationsQuery.data.some((organization) => organization.id === selectedOrganizationID)
    ) {
      setSelectedOrganizationID(organizationsQuery.data[0].id);
      setSelectedProjectID(undefined);
      clearConversation();
    }
  }, [organizationsQuery.data, selectedOrganizationID]);

  useEffect(() => {
    if (!selectedProjectID && projectsQuery.data && projectsQuery.data.length > 0) {
      setSelectedProjectID(projectsQuery.data[0].id);
    }
  }, [projectsQuery.data, selectedProjectID]);

  useEffect(() => {
    if (
      selectedProjectID &&
      projectsQuery.data &&
      projectsQuery.data.length > 0 &&
      !projectsQuery.data.some((project) => project.id === selectedProjectID)
    ) {
      setSelectedProjectID(projectsQuery.data[0].id);
      clearConversation();
    }
  }, [projectsQuery.data, selectedProjectID]);

  useEffect(() => {
    if (!selectedChannelID && channelsQuery.data && channelsQuery.data.length > 0) {
      selectChannel(channelsQuery.data[0]);
    }
  }, [channelsQuery.data, selectedChannelID]);

  useEffect(() => {
    if (!activeConversation && selectedChannel?.type === "text") {
      setActiveConversation({
        type: "channel",
        id: selectedChannel.id,
        projectID: selectedChannel.project_id,
        channelID: selectedChannel.id
      });
    }
  }, [activeConversation, selectedChannel]);

  useEffect(() => {
    if (activeConversation?.type !== "thread" || !threadsQuery.data) {
      return;
    }
    if (threadsQuery.data.some((thread) => thread.id === activeConversation.id)) {
      return;
    }
    clearActiveConversation();
  }, [activeConversation, threadsQuery.data]);

  useEffect(() => {
    if (
      selectedChannelID &&
      channelsQuery.data &&
      channelsQuery.data.length > 0 &&
      !channelsQuery.data.some((channel) => channel.id === selectedChannelID)
    ) {
      selectChannel(channelsQuery.data[0]);
    }
  }, [channelsQuery.data, selectedChannelID]);

  const prevConversationKeyRef = useRef<string>("");
  useEffect(() => {
    const nextKey = conversationKey(activeConversation);
    const prevKey = prevConversationKeyRef.current;
    if (prevKey && prevKey !== nextKey) {
      setStreamingByRunID((current) => {
        if (Object.keys(current).length > 0) {
          streamingCacheRef.current[prevKey] = current;
        } else {
          delete streamingCacheRef.current[prevKey];
        }
        return {};
      });
      setPendingQuestion((current) => {
        pendingQuestionCacheRef.current[prevKey] = current;
        return null;
      });
      setQueuedPrompts((current) => {
        if (current.length > 0) {
          queuedPromptsCacheRef.current[prevKey] = current;
        } else {
          delete queuedPromptsCacheRef.current[prevKey];
        }
        return [];
      });
    }
    prevConversationKeyRef.current = nextKey;
    setConversationMessages([]);
    setMessagesLoading(Boolean(activeConversation));
    setOlderMessagesLoading(false);
    setMessageHistoryHasMore(false);
    const cached = streamingCacheRef.current[nextKey];
    if (cached) {
      setStreamingByRunID(cached);
    } else {
      setStreamingByRunID({});
    }
    const cachedQuestion = pendingQuestionCacheRef.current[nextKey];
    setPendingQuestion(cachedQuestion ?? null);
    setQueuedPrompts(queuedPromptsCacheRef.current[nextKey] ?? []);
  }, [conversationKey(activeConversation)]);

  useEffect(() => {
    if (meQuery.isError) {
      clearSession();
    }
  }, [meQuery.isError]);

  const invalidateAgentConfigQueries = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["agents", selectedOrganizationID] });
    void queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
  }, [queryClient, selectedOrganizationID]);

  const handleSocketEvent = useCallback(
    (event: AgentXEvent) => {
      if (
        !activeConversation ||
        !eventMatchesActiveConversation(event, {
          organizationID: selectedOrganizationID,
          conversationType: activeConversation.type,
          conversationID: activeConversation.id
        })
      ) {
        return;
      }

      switch (event.type) {
        case "MessageHistoryStarted":
          if (event.payload.before) {
            setOlderMessagesLoading(true);
          } else {
            setMessagesLoading((current) => messageHistoryLoadingForEvent(current, event));
          }
          break;
        case "MessageHistoryChunk": {
          const activeMessages = event.payload.messages.filter((message) =>
            messageMatchesActiveConversation(message, {
              organizationID: selectedOrganizationID,
              conversationType: activeConversation.type,
              conversationID: activeConversation.id
            })
          );
          setConversationMessages((current) => mergeMessages(current, activeMessages));
          if (activeMessages.some((m) => m.sender_type === "bot")) {
            const activeAgents =
              conversationContextQuery.data?.agents ?? channelAgentsQuery.data ?? [];
            setStreamingByRunID((current) => {
              const next = { ...current };
              for (const [runID, item] of Object.entries(next)) {
                if (streamingRunHasCompletedMessage(item, activeMessages, activeAgents)) {
                  delete next[runID];
                }
              }
              return next;
            });
          }
          break;
        }
        case "MessageHistoryCompleted":
          setMessageHistoryHasMore(event.payload.has_more);
          if (event.payload.before) {
            setOlderMessagesLoading(false);
          } else {
            setMessagesLoading((current) => messageHistoryLoadingForEvent(current, event));
          }
          break;
        case "MessageCreated": {
          const message = event.payload.message;
          setConversationMessages((current) => mergeMessages(current, [message]));
          if (isSystemCommandMessage(message)) {
            invalidateAgentConfigQueries();
          }
          if (message.sender_type === "bot") {
            showAgentMessageNotification(message);
            setStreamingByRunID((current) => {
              const next = { ...current };
              for (const [runID, item] of Object.entries(next)) {
                if (!item.error) {
                  delete next[runID];
                  break;
                }
              }
              return next;
            });
          }
          break;
        }
        case "MessageUpdated":
          setConversationMessages((current) => mergeMessages(current, [event.payload.message]));
          break;
        case "MessageDeleted":
          setConversationMessages((current) =>
            removeMessageAndMarkReferencesDeleted(current, event.payload.message_id)
          );
          break;
        case "AgentPromptQueued":
          setQueuedPrompts((current) =>
            upsertQueuedPrompt(current, {
              queueID: event.payload.queue_id,
              messageID: event.payload.message_id,
              agentID: event.payload.agent_id,
              body: event.payload.body,
              createdAt: event.payload.created_at,
              canSteer: event.payload.can_steer
            })
          );
          break;
        case "AgentPromptQueueRemoved":
          setQueuedPrompts((current) =>
            current.filter((item) => item.queueID !== event.payload.queue_id)
          );
          break;
        case "AgentRunStarted":
          setStreamingByRunID((current) => ({
            ...current,
            [event.payload.run_id]: {
              runID: event.payload.run_id,
              agentID: event.payload.agent_id,
              startedAt: event.created_at,
              text: "",
              team: event.payload.team
            }
          }));
          break;
        case "AgentOutputDelta":
          setStreamingByRunID((current) => {
            const existing = current[event.payload.run_id];
            return {
              ...current,
              [event.payload.run_id]: {
                runID: event.payload.run_id,
                agentID: event.payload.agent_id ?? existing?.agentID,
                startedAt: existing?.startedAt ?? event.created_at,
                text: `${event.payload.clear_text ? "" : (existing?.text ?? "")}${event.payload.text}`,
                team: event.payload.team ?? existing?.team,
                thinking: event.payload.thinking
                  ? `${existing?.thinking ?? ""}${event.payload.thinking}`
                  : existing?.thinking,
                process:
                  event.payload.process && event.payload.process.length > 0
                    ? [...(existing?.process ?? []), ...event.payload.process]
                    : existing?.process
              }
            };
          });
          break;
        case "AgentRunCompleted":
          setPendingQuestion((current) =>
            current?.runID === event.payload.run_id ? null : current
          );
          setStreamingByRunID((current) => {
            const next = { ...current };
            delete next[event.payload.run_id];
            return next;
          });
          break;
        case "AgentRunCanceled":
          setPendingQuestion((current) =>
            current?.runID === event.payload.run_id ? null : current
          );
          setStreamingByRunID((current) => {
            const next = { ...current };
            delete next[event.payload.run_id];
            return next;
          });
          break;
        case "AgentRunFailed":
          setPendingQuestion((current) =>
            current?.runID === event.payload.run_id ? null : current
          );
          setStreamingByRunID((current) => {
            // When the failure was persisted as a message, the error reason
            // already arrived via MessageCreated; drop the streaming placeholder
            // so the run shows a single, refresh-safe error message.
            if (event.payload.persisted) {
              if (!(event.payload.run_id in current)) {
                return current;
              }
              const next = { ...current };
              delete next[event.payload.run_id];
              return next;
            }
            const existing = current[event.payload.run_id];
            return {
              ...current,
              [event.payload.run_id]: {
                runID: event.payload.run_id,
                agentID: event.payload.agent_id ?? existing?.agentID,
                startedAt: existing?.startedAt ?? event.created_at,
                endedAt: event.created_at,
                text: existing?.text ?? "",
                thinking: existing?.thinking,
                process: existing?.process,
                team: event.payload.team ?? existing?.team,
                error: event.payload.error || "Agent run failed"
              }
            };
          });
          break;
        case "AgentInputRequest":
          showAgentInputRequestNotification(
            event.payload,
            agentNameForID(event.payload.agent_id, conversationContextQuery.data?.agents, agentsQuery.data)
          );
          setPendingQuestion({
            runID: event.payload.run_id,
            agentID: event.payload.agent_id,
            questionID: event.payload.question_id,
            question: event.payload.question,
            options: event.payload.options
          });
          break;
      }
    },
    [
      selectedOrganizationID,
      activeConversation,
      conversationContextQuery.data,
      channelAgentsQuery.data,
      agentsQuery.data,
      invalidateAgentConfigQueries
    ]
  );

  const {
    connectionStatus,
    loadOlderMessages: requestOlderMessages,
  } = useConversationSocket(
    selectedOrganizationID,
    activeConversation?.type,
    activeConversation?.id,
    handleSocketEvent
  );

  const handleLoadOlderMessages = useCallback((): boolean => {
    const oldestMessage = conversationMessages[0];
    if (
      !oldestMessage ||
      !messageHistoryHasMore ||
      messagesLoading ||
      olderMessagesLoading ||
      !requestOlderMessages(oldestMessage.created_at)
    ) {
      return false;
    }
    setOlderMessagesLoading(true);
    return true;
  }, [
    conversationMessages,
    messageHistoryHasMore,
    messagesLoading,
    olderMessagesLoading,
    requestOlderMessages
  ]);

  function handleAuthenticated(result: AuthResponse) {
    setSessionToken(result.session_token);
    setSelectedOrganizationID(undefined);
    setSelectedProjectID(undefined);
    setSelectedChannelID(undefined);
    setActiveConversation(undefined);
    setConversationMessages([]);
    setMessagesLoading(false);
    setOlderMessagesLoading(false);
    setMessageHistoryHasMore(false);
    setStreamingByRunID({});
    setQueuedPrompts([]);
    void queryClient.invalidateQueries();
  }

  function clearSession() {
    clearToken();
    clearPageSelection();
    setSessionToken(null);
    setSelectedOrganizationID(undefined);
    setSelectedProjectID(undefined);
    setSelectedChannelID(undefined);
    setActiveConversation(undefined);
    setConversationMessages([]);
    setMessagesLoading(false);
    setOlderMessagesLoading(false);
    setMessageHistoryHasMore(false);
    setStreamingByRunID({});
    setQueuedPrompts([]);
    queryClient.clear();
  }

  async function handleLogout() {
    try {
      await logoutRequest();
    } finally {
      clearSession();
    }
  }

  function clearConversation() {
    setSelectedChannelID(undefined);
    clearActiveConversation();
  }

  function clearActiveConversation() {
    setActiveConversation(undefined);
    setConversationMessages([]);
    setMessagesLoading(false);
    setOlderMessagesLoading(false);
    setMessageHistoryHasMore(false);
    setStreamingByRunID({});
    setQueuedPrompts([]);
  }

  function handleSelectOrganization(orgID: string) {
    if (orgID === selectedOrganizationID) {
      return;
    }
    setSelectedOrganizationID(orgID);
    setSelectedProjectID(undefined);
    clearConversation();
  }

  function handleSelectProject(projectID: string) {
    if (projectID === selectedProjectID) {
      return;
    }
    setSelectedProjectID(projectID);
    clearConversation();
  }

  function selectChannel(channel: Channel) {
    const nextConversation =
      channel.type === "text"
        ? {
            type: "channel" as const,
            id: channel.id,
            projectID: channel.project_id,
            channelID: channel.id
          }
        : undefined;
    const currentKey = conversationKey(activeConversation);
    const nextKey = conversationKey(nextConversation);

    setSelectedChannelID(channel.id);
    if (currentKey === nextKey) {
      return;
    }
    setConversationMessages([]);
    setActiveConversation(nextConversation);
    if (!nextConversation) {
      setMessagesLoading(false);
      setOlderMessagesLoading(false);
      setMessageHistoryHasMore(false);
    }
  }

  function handleSelectThread(thread: Thread) {
    setSelectedChannelID(thread.channel_id);
    setActiveConversation({
      type: "thread",
      id: thread.id,
      projectID: thread.project_id,
      channelID: thread.channel_id
    });
  }

  function handleMessageSent(message: Message) {
    if (
      !activeConversation ||
      !messageMatchesActiveConversation(message, {
        organizationID: selectedOrganizationID,
        conversationType: activeConversation.type,
        conversationID: activeConversation.id
      })
    ) {
      return;
    }
    setConversationMessages((current) => mergeMessages(current, [message]));
    if (isSystemCommandMessage(message)) {
      invalidateAgentConfigQueries();
    }
  }

  async function handleCreateProject(payload: { name: string; workspace_path?: string }): Promise<Project> {
    const created = await createProject(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({ queryKey: ["projects", selectedOrganizationID] });
    return created;
  }

  async function handleUpdateProject(
    projectID: string,
    payload: { name?: string; workspace_path?: string }
  ): Promise<Project> {
    const updated = await updateProject(projectID, payload);
    await queryClient.invalidateQueries({ queryKey: ["projects", selectedOrganizationID] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
    await queryClient.invalidateQueries({ queryKey: ["channel-agents", selectedChannelID] });
    await queryClient.invalidateQueries({ queryKey: ["workspace", updated.workspace_id] });
    return updated;
  }

  async function handleDeleteProject(project: Project): Promise<void> {
    await deleteProject(project.id);
    await queryClient.invalidateQueries({ queryKey: ["projects", selectedOrganizationID] });
    await queryClient.invalidateQueries({ queryKey: ["project-channels", project.id] });
    if (project.id === selectedProjectID) {
      const nextProject = projectsQuery.data?.find((item) => item.id !== project.id);
      setSelectedProjectID(nextProject?.id);
      clearConversation();
    }
  }

  async function handleCreateChannel(
    name: string,
    type: Channel["type"],
    budget?: Pick<Channel, "team_max_batches" | "team_max_runs">
  ): Promise<Channel> {
    const created = await createChannel(selectedProjectID as string, name, type, budget);
    await queryClient.invalidateQueries({ queryKey: ["project-channels", selectedProjectID] });
    return created;
  }

  async function handleUpdateChannel(
    channelID: string,
    name: string,
    budget?: Pick<Channel, "team_max_batches" | "team_max_runs">
  ): Promise<Channel> {
    const updated = await updateChannel(channelID, { name, ...budget });
    await queryClient.invalidateQueries({ queryKey: ["project-channels", selectedProjectID] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
    return updated;
  }

  async function handleDeleteChannel(channel: Channel): Promise<void> {
    await deleteChannel(channel.id);
    await queryClient.invalidateQueries({ queryKey: ["project-channels", selectedProjectID] });
    if (channel.id === selectedChannelID) {
      clearConversation();
    }
  }

  async function handleCreateThread(title: string, body: string): Promise<CreateThreadResponse> {
    const created = await createThread(selectedChannelID as string, title, body);
    await queryClient.invalidateQueries({ queryKey: ["threads", selectedChannelID] });
    return created;
  }

  async function handleUpdateThread(threadID: string, title: string): Promise<Thread> {
    const updated = await updateThread(threadID, title);
    await queryClient.invalidateQueries({ queryKey: ["threads", updated.channel_id] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
    return updated;
  }

  async function handleDeleteThread(thread: Thread): Promise<void> {
    await deleteThread(thread.id);
    await queryClient.invalidateQueries({ queryKey: ["threads", thread.channel_id] });
    if (activeConversation?.type === "thread" && activeConversation.id === thread.id) {
      setSelectedChannelID(thread.channel_id);
      setActiveConversation(undefined);
      setConversationMessages([]);
      setMessagesLoading(false);
      setOlderMessagesLoading(false);
      setMessageHistoryHasMore(false);
      setStreamingByRunID({});
      setQueuedPrompts([]);
    }
  }

  async function handleUpdateMessage(messageID: string, body: string): Promise<Message> {
    const updated = await updateMessage(messageID, body);
    setConversationMessages((current) => mergeMessages(current, [updated]));
    return updated;
  }

  async function handleDeleteMessage(message: Message): Promise<void> {
    await deleteMessage(message.id);
    setConversationMessages((current) => removeMessageAndMarkReferencesDeleted(current, message.id));
  }

  async function handleRetryMessage(message: Message): Promise<void> {
    if (!activeConversation) return;
    const agents = conversationContextQuery.data?.agents ?? channelAgentsQuery.data ?? [];
    const agent = agents.find((item) => item.agent.bot_user_id === message.sender_id)?.agent;
    if (!agent) {
      throw new Error("Could not resolve the agent for this message");
    }
    // The stale reply is removed and the fresh run streams in via websocket events.
    await retryAgentRun(activeConversation.type, activeConversation.id, agent.id);
  }

  async function handleRespondToQuestion(questionID: string, answer: string) {
    if (!activeConversation) return;
    await respondToInputRequest(
      activeConversation.type,
      activeConversation.id,
      questionID,
      answer
    );
    setPendingQuestion(null);
  }

  async function handleSteerQueuedPrompt(queueID: string) {
    if (!activeConversation) return;
    await steerQueuedPrompt(activeConversation.type, activeConversation.id, queueID);
  }

  async function handleDeleteQueuedPrompt(queueID: string) {
    if (!activeConversation) return;
    await deleteQueuedPrompt(activeConversation.type, activeConversation.id, queueID);
  }

  async function handleSaveChannelAgents(
    bindings: Array<{ agent_id: string; run_workspace_id?: string }>
  ) {
    if (!selectedChannelID) {
      return;
    }
    await setChannelAgents(selectedChannelID, bindings);
    await queryClient.invalidateQueries({ queryKey: ["channel-agents", selectedChannelID] });
    await queryClient.invalidateQueries({ queryKey: ["agent-channels"] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
  }

  async function handleCreateAgent(payload: {
    name: string;
    description?: string;
    handle?: string;
    kind?: string;
    model?: string;
    effort?: string;
    fast_mode?: boolean;
    yolo_mode?: boolean;
    env?: Record<string, string>;
  }): Promise<Agent> {
    const created = await createAgent(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({ queryKey: ["agents", selectedOrganizationID] });
    return created;
  }

  async function handleUpdateAgent(
    agentID: string,
    payload: Partial<Pick<Agent, "name" | "description" | "handle" | "kind" | "model" | "effort" | "enabled" | "fast_mode" | "yolo_mode">> & {
      env?: Record<string, string>;
    }
  ) {
    await updateAgent(agentID, payload);
    await queryClient.invalidateQueries({ queryKey: ["agents", selectedOrganizationID] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
  }

  async function handleDeleteAgent(agentID: string) {
    await deleteAgent(agentID);
    await queryClient.invalidateQueries({ queryKey: ["agents", selectedOrganizationID] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
  }

  async function handleUpdateNotificationSettings(payload: {
    webhook_enabled: boolean;
    webhook_url: string;
    webhook_secret?: string;
  }): Promise<NotificationSettings> {
    const updated = await updateNotificationSettings(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({
      queryKey: ["notification-settings", selectedOrganizationID]
    });
    return updated;
  }

  async function handleUpdateServerSettings(payload: ServerSettingsUpdatePayload): Promise<ServerSettings> {
    const updated = await updateServerSettings(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({
      queryKey: ["server-settings", selectedOrganizationID]
    });
    return updated;
  }

  async function handleRestartServer(): Promise<void> {
    await restartServer(selectedOrganizationID as string);
  }

  async function handleUpdateToolUpdateSettings(payload: ToolUpdateSettings): Promise<ToolUpdateOverview> {
    const updated = await updateToolUpdateSettings(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({ queryKey: ["tool-updates", selectedOrganizationID] });
    return updated;
  }

  async function handleCheckToolUpdates(tool: ToolUpdateStatus["tool"] | "all"): Promise<ToolUpdateOverview> {
    const updated = await checkToolUpdates(selectedOrganizationID as string, tool);
    await queryClient.invalidateQueries({ queryKey: ["tool-updates", selectedOrganizationID] });
    return updated;
  }

  async function handleRunToolUpdate(tool: ToolUpdateStatus["tool"] | "all"): Promise<ToolUpdateOverview> {
    const updated = await runToolUpdate(selectedOrganizationID as string, tool);
    await queryClient.invalidateQueries({ queryKey: ["tool-updates", selectedOrganizationID] });
    return updated;
  }

  async function handleUpdateSelfUpdateSettings(payload: SelfUpdateSettings): Promise<SelfUpdateOverview> {
    const updated = await updateSelfUpdateSettings(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({ queryKey: ["self-update", selectedOrganizationID] });
    return updated;
  }

  async function handleCheckSelfUpdate(): Promise<SelfUpdateOverview> {
    try {
      return await checkSelfUpdate(selectedOrganizationID as string);
    } finally {
      await queryClient.invalidateQueries({ queryKey: ["self-update", selectedOrganizationID] });
    }
  }

  async function handleRunSelfUpdate(): Promise<SelfUpdateOverview> {
    try {
      return await runSelfUpdate(selectedOrganizationID as string);
    } finally {
      await queryClient.invalidateQueries({ queryKey: ["self-update", selectedOrganizationID] });
    }
  }

  async function handleUpdateUserPreferences(payload: UserPreferences): Promise<UserPreferences> {
    const updated = await updateUserPreferences(payload);
    await queryClient.invalidateQueries({ queryKey: ["user-preferences", sessionToken] });
    return updated;
  }

  async function handleTestNotificationSettings(): Promise<void> {
    await testNotificationSettings(selectedOrganizationID as string);
  }


  const handleToggleTheme = useCallback(() => {
    setTheme((current) => (current === "dark" ? "light" : "dark"));
  }, []);

  if (!hasSession) {
    return <LoginView onAuthenticated={handleAuthenticated} />;
  }

  if (!meQuery.data) {
    return <LoadingSessionView onClearSession={clearSession} />;
  }

  return (
    <Shell
      user={meQuery.data}
      organization={selectedOrganization}
      projects={projectsQuery.data ?? []}
      project={selectedProject}
      projectWorkspace={selectedProjectWorkspaceQuery.data}
      channels={channelsQuery.data ?? []}
      selectedChannel={selectedChannel}
      activeConversation={activeConversation}
      threads={threadsQuery.data ?? []}
      agents={agentsQuery.data ?? []}
      channelAgents={channelAgentsQuery.data ?? []}
      conversationContext={conversationContextQuery.data}
      contextLoading={conversationContextQuery.isLoading}
      messages={conversationMessages}
      messagesLoading={messagesLoading}
      olderMessagesLoading={olderMessagesLoading}
      hasOlderMessages={messageHistoryHasMore}
      streaming={Object.values(streamingByRunID)}
      pendingQuestion={pendingQuestion}
      queuedPrompts={queuedPrompts}
      onRespondToQuestion={handleRespondToQuestion}
      onSteerQueuedPrompt={handleSteerQueuedPrompt}
      onDeleteQueuedPrompt={handleDeleteQueuedPrompt}
      connectionStatus={connectionStatus}
      notificationSettings={notificationSettingsQuery.data}
      notificationSettingsLoading={notificationSettingsQuery.isLoading}
      serverSettings={serverSettingsQuery.data}
      serverSettingsLoading={serverSettingsQuery.isLoading}
      serverSettingsError={serverSettingsQuery.error instanceof Error ? serverSettingsQuery.error.message : null}
      toolUpdates={toolUpdatesQuery.data}
      toolUpdatesLoading={toolUpdatesQuery.isLoading}
      selfUpdate={selfUpdateQuery.data}
      selfUpdateLoading={selfUpdateQuery.isLoading}
      preferences={userPreferencesQuery.data ?? { show_ttft: true, show_tps: true, hide_avatars: false }}
      preferencesLoading={userPreferencesQuery.isLoading}
      theme={theme}
      onSelectProject={handleSelectProject}
      onCreateProject={handleCreateProject}
      onUpdateProject={handleUpdateProject}
      onDeleteProject={handleDeleteProject}
      onSelectChannel={selectChannel}
      onCreateChannel={handleCreateChannel}
      onUpdateChannel={handleUpdateChannel}
      onDeleteChannel={handleDeleteChannel}
      onSelectThread={handleSelectThread}
      onCreateThread={handleCreateThread}
      onUpdateThread={handleUpdateThread}
      onDeleteThread={handleDeleteThread}
      onSaveChannelAgents={handleSaveChannelAgents}
      onCreateAgent={handleCreateAgent}
      onUpdateAgent={handleUpdateAgent}
      onDeleteAgent={handleDeleteAgent}
      onUpdateNotificationSettings={handleUpdateNotificationSettings}
      onUpdateServerSettings={handleUpdateServerSettings}
      onRestartServer={handleRestartServer}
      onUpdateToolUpdateSettings={handleUpdateToolUpdateSettings}
      onCheckToolUpdates={handleCheckToolUpdates}
      onRunToolUpdate={handleRunToolUpdate}
      onUpdateSelfUpdateSettings={handleUpdateSelfUpdateSettings}
      onCheckSelfUpdate={handleCheckSelfUpdate}
      onRunSelfUpdate={handleRunSelfUpdate}
      onUpdateUserPreferences={handleUpdateUserPreferences}
      onTestNotificationSettings={handleTestNotificationSettings}
      onLoadWorkspaceTree={handleLoadWorkspaceTree}
      onSearchWorkspace={handleSearchWorkspace}
      onReadWorkspaceFile={handleReadWorkspaceFile}
      onFetchWorkspaceFileBlob={handleFetchWorkspaceFileBlob}
      onWriteWorkspaceFile={handleWriteWorkspaceFile}
      onDeleteWorkspaceFile={handleDeleteWorkspaceFile}
      onCreateWorkspaceEntry={handleCreateWorkspaceEntry}
      onMoveWorkspaceEntry={handleMoveWorkspaceEntry}
      onDeleteWorkspaceEntry={handleDeleteWorkspaceEntry}
      onLoadWorkspaceGitStatus={handleLoadWorkspaceGitStatus}
      onLoadWorkspaceGitHistory={handleLoadWorkspaceGitHistory}
      onLoadWorkspaceGitDiff={handleLoadWorkspaceGitDiff}
      onUpdateMessage={handleUpdateMessage}
      onDeleteMessage={handleDeleteMessage}
      onRetryMessage={handleRetryMessage}
      onLoadOlderMessages={handleLoadOlderMessages}
      onMessageSent={handleMessageSent}
      onToggleTheme={handleToggleTheme}
      onLogout={handleLogout}
    />
  );
}
