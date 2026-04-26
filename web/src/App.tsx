import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  agents,
  channelAgents,
  channelThreads,
  clearToken,
  conversationContext,
  createAgent,
  createChannel,
  createProject,
  createThread,
  deleteAgent,
  deleteChannel,
  deleteMessage,
  deleteThread,
  getToken,
  me,
  organizations,
  projectChannels,
  projects,
  putWorkspaceFile,
  setChannelAgents,
  updateAgent,
  updateChannel,
  updateMessage,
  updateProject,
  updateThread,
  workspace,
  workspaceFile,
  workspaceTree
} from "./api/client";
import type {
  Agent,
  AuthResponse,
  Channel,
  ConversationAgentContext,
  ConversationType,
  CreateThreadResponse,
  Message,
  ProcessItem,
  Project,
  Thread,
  WorkspaceTreeEntry
} from "./api/types";
import { LoginView } from "./components/LoginView";
import { Shell } from "./components/Shell";
import {
  eventMatchesActiveConversation,
  mergeMessages,
  messageHistoryLoadingForEvent,
  messageMatchesActiveConversation,
  streamingRunHasCompletedMessage
} from "./messages/state";
import type { AgentXEvent } from "./ws/events";
import { useConversationSocket } from "./ws/useConversationSocket";
import { applyTheme, getInitialTheme, storeTheme, type ThemeMode } from "./theme";

interface ActiveConversation {
  type: ConversationType;
  id: string;
  projectID: string;
  channelID: string;
}

interface StreamingMessage {
  runID: string;
  agentID?: string;
  startedAt?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
}

function conversationKey(conversation?: ActiveConversation): string {
  return conversation ? `${conversation.type}:${conversation.id}` : "";
}

export default function App() {
  const queryClient = useQueryClient();
  const [sessionToken, setSessionToken] = useState(() => getToken());
  const [selectedOrganizationID, setSelectedOrganizationID] = useState<string>();
  const [selectedProjectID, setSelectedProjectID] = useState<string>();
  const [selectedChannelID, setSelectedChannelID] = useState<string>();
  const [activeConversation, setActiveConversation] = useState<ActiveConversation>();
  const [conversationMessages, setConversationMessages] = useState<Message[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [olderMessagesLoading, setOlderMessagesLoading] = useState(false);
  const [messageHistoryHasMore, setMessageHistoryHasMore] = useState(false);
  const [streamingByRunID, setStreamingByRunID] = useState<Record<string, StreamingMessage>>({});
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme());
  const streamingCacheRef = useRef<Record<string, Record<string, StreamingMessage>>>({});
  const hasSession = Boolean(sessionToken);

  useEffect(() => {
    applyTheme(theme);
    storeTheme(theme);
  }, [theme]);

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
  }, [conversationKey(activeConversation)]);

  useEffect(() => {
    if (meQuery.isError) {
      clearSession();
    }
  }, [meQuery.isError]);

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
          if (message.sender_type === "bot") {
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
            current.filter((message) => message.id !== event.payload.message_id)
          );
          break;
        case "AgentRunStarted":
          setStreamingByRunID((current) => ({
            ...current,
            [event.payload.run_id]: {
              runID: event.payload.run_id,
              agentID: event.payload.agent_id,
              startedAt: event.created_at,
              text: ""
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
                text: `${existing?.text ?? ""}${event.payload.text}`,
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
          setStreamingByRunID((current) => {
            const next = { ...current };
            delete next[event.payload.run_id];
            return next;
          });
          break;
        case "AgentRunFailed":
          setStreamingByRunID((current) => ({
            ...current,
            [event.payload.run_id]: {
              runID: event.payload.run_id,
              startedAt: event.created_at,
              text: "",
              error: event.payload.error || "Agent run failed"
            }
          }));
          break;
      }
    },
    [
      selectedOrganizationID,
      activeConversation,
      conversationContextQuery.data,
      channelAgentsQuery.data
    ]
  );

  const requestOlderMessages = useConversationSocket(
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
    void queryClient.invalidateQueries();
  }

  function clearSession() {
    clearToken();
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
    queryClient.clear();
  }

  function clearConversation() {
    setSelectedChannelID(undefined);
    setActiveConversation(undefined);
    setConversationMessages([]);
    setMessagesLoading(false);
    setOlderMessagesLoading(false);
    setMessageHistoryHasMore(false);
    setStreamingByRunID({});
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
    setSelectedChannelID(channel.id);
    setConversationMessages([]);
    if (channel.type === "text") {
      setActiveConversation({
        type: "channel",
        id: channel.id,
        projectID: channel.project_id,
        channelID: channel.id
      });
    } else {
      setActiveConversation(undefined);
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
  }

  async function handleCreateProject(name: string): Promise<Project> {
    const created = await createProject(selectedOrganizationID as string, name);
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

  async function handleCreateChannel(name: string, type: Channel["type"]): Promise<Channel> {
    const created = await createChannel(selectedProjectID as string, name, type);
    await queryClient.invalidateQueries({ queryKey: ["project-channels", selectedProjectID] });
    return created;
  }

  async function handleUpdateChannel(channelID: string, name: string): Promise<Channel> {
    const updated = await updateChannel(channelID, { name });
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
    }
  }

  async function handleUpdateMessage(messageID: string, body: string): Promise<Message> {
    const updated = await updateMessage(messageID, body);
    setConversationMessages((current) => mergeMessages(current, [updated]));
    return updated;
  }

  async function handleDeleteMessage(message: Message): Promise<void> {
    await deleteMessage(message.id);
    setConversationMessages((current) => current.filter((item) => item.id !== message.id));
  }

  async function handleSaveChannelAgents(
    bindings: Array<{ agent_id: string; run_workspace_id?: string }>
  ) {
    if (!selectedChannelID) {
      return;
    }
    await setChannelAgents(selectedChannelID, bindings);
    await queryClient.invalidateQueries({ queryKey: ["channel-agents", selectedChannelID] });
    await queryClient.invalidateQueries({ queryKey: ["conversation-context"] });
  }

  async function handleCreateAgent(payload: {
    name: string;
    handle?: string;
    kind?: string;
    model?: string;
    yolo_mode?: boolean;
    env?: Record<string, string>;
  }): Promise<Agent> {
    const created = await createAgent(selectedOrganizationID as string, payload);
    await queryClient.invalidateQueries({ queryKey: ["agents", selectedOrganizationID] });
    return created;
  }

  async function handleUpdateAgent(
    agentID: string,
    payload: Partial<Pick<Agent, "name" | "handle" | "kind" | "model" | "enabled" | "yolo_mode">> & {
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

  async function handleReadWorkspaceFile(workspaceID: string, path: string): Promise<string> {
    const file = await workspaceFile(workspaceID, path);
    return file.body;
  }

  async function handleLoadWorkspaceTree(workspaceID: string): Promise<WorkspaceTreeEntry> {
    return workspaceTree(workspaceID);
  }

  async function handleWriteWorkspaceFile(workspaceID: string, path: string, body: string) {
    await putWorkspaceFile(workspaceID, path, body);
  }

  const handleToggleTheme = useCallback(() => {
    setTheme((current) => (current === "dark" ? "light" : "dark"));
  }, []);

  if (!hasSession) {
    return <LoginView onAuthenticated={handleAuthenticated} />;
  }

  if (!meQuery.data) {
    return (
      <main className="flex h-screen w-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-4">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-lg">
            AX
          </div>
          <span className="text-sm text-muted-foreground">Loading session...</span>
          <button
            className="text-sm text-muted-foreground hover:text-foreground underline"
            type="button"
            onClick={clearSession}
          >
            Clear session
          </button>
        </div>
      </main>
    );
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
      theme={theme}
      onSelectProject={handleSelectProject}
      onCreateProject={handleCreateProject}
      onUpdateProject={handleUpdateProject}
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
      onLoadWorkspaceTree={handleLoadWorkspaceTree}
      onReadWorkspaceFile={handleReadWorkspaceFile}
      onWriteWorkspaceFile={handleWriteWorkspaceFile}
      onUpdateMessage={handleUpdateMessage}
      onDeleteMessage={handleDeleteMessage}
      onLoadOlderMessages={handleLoadOlderMessages}
      onMessageSent={handleMessageSent}
      onToggleTheme={handleToggleTheme}
      onLogout={clearSession}
    />
  );
}
