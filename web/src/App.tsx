import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { clearToken, getToken, me, organizations, channels, messages } from "./api/client";
import type { BootstrapResponse, Channel, Message } from "./api/types";
import { LoginView } from "./components/LoginView";
import { Shell } from "./components/Shell";
import {
  eventMatchesActiveConversation,
  mergeMessages,
  messageMatchesActiveConversation
} from "./messages/state";
import type { AgentXEvent } from "./ws/events";
import { useConversationSocket } from "./ws/useConversationSocket";

interface StreamingMessage {
  runID: string;
  text: string;
  error?: string;
}

export default function App() {
  const queryClient = useQueryClient();
  const [sessionToken, setSessionToken] = useState(() => getToken());
  const [selectedOrganizationID, setSelectedOrganizationID] = useState<string>();
  const [selectedChannelID, setSelectedChannelID] = useState<string>();
  const [conversationMessages, setConversationMessages] = useState<Message[]>([]);
  const [streaming, setStreaming] = useState<StreamingMessage | null>(null);
  const hasSession = Boolean(sessionToken);

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

  const channelsQuery = useQuery({
    queryKey: ["channels", selectedOrganizationID],
    queryFn: () => channels(selectedOrganizationID as string),
    enabled: hasSession && Boolean(selectedOrganizationID)
  });

  const selectedChannel = useMemo(
    () => channelsQuery.data?.find((channel) => channel.id === selectedChannelID),
    [channelsQuery.data, selectedChannelID]
  );

  const messagesQuery = useQuery({
    queryKey: ["messages", "channel", selectedChannelID],
    queryFn: () => messages("channel", selectedChannelID as string),
    enabled: hasSession && Boolean(selectedChannelID)
  });

  useEffect(() => {
    if (!selectedOrganizationID && organizationsQuery.data && organizationsQuery.data.length > 0) {
      setSelectedOrganizationID(organizationsQuery.data[0].id);
    }
  }, [organizationsQuery.data, selectedOrganizationID]);

  useEffect(() => {
    if (!selectedChannelID && channelsQuery.data && channelsQuery.data.length > 0) {
      setSelectedChannelID(channelsQuery.data[0].id);
    }
  }, [channelsQuery.data, selectedChannelID]);

  useEffect(() => {
    if (messagesQuery.data) {
      const active = {
        organizationID: selectedOrganizationID,
        conversationID: selectedChannelID
      };
      const activeMessages = messagesQuery.data.filter((message) =>
        messageMatchesActiveConversation(message, active)
      );
      setConversationMessages((current) => mergeMessages(current, activeMessages));
    }
  }, [messagesQuery.data, selectedOrganizationID, selectedChannelID]);

  useEffect(() => {
    if (meQuery.isError) {
      clearSession();
    }
  }, [meQuery.isError]);

  const handleSocketEvent = useCallback((event: AgentXEvent) => {
    if (
      !eventMatchesActiveConversation(event, {
        organizationID: selectedOrganizationID,
        conversationID: selectedChannelID
      })
    ) {
      return;
    }

    switch (event.type) {
      case "MessageCreated": {
        const message = event.payload.message;
        setConversationMessages((current) => mergeMessages(current, [message]));
        if (message.sender_type === "bot") {
          setStreaming(null);
        }
        break;
      }
      case "AgentRunStarted":
        setStreaming({ runID: event.payload.run_id, text: "" });
        break;
      case "AgentOutputDelta":
        setStreaming((current) => {
          if (!current || current.runID !== event.payload.run_id) {
            return { runID: event.payload.run_id, text: event.payload.text };
          }
          return { ...current, text: current.text + event.payload.text };
        });
        break;
      case "AgentRunCompleted":
        setStreaming((current) =>
          current?.runID === event.payload.run_id ? null : current
        );
        break;
      case "AgentRunFailed":
        setStreaming({
          runID: event.payload.run_id,
          text: "",
          error: event.payload.error || "Agent run failed"
        });
        break;
    }
  }, [selectedOrganizationID, selectedChannelID]);

  useConversationSocket(selectedOrganizationID, selectedChannelID, handleSocketEvent);

  function handleBootstrap(result: BootstrapResponse) {
    setSessionToken(result.session_token);
    setSelectedOrganizationID(result.organization.id);
    setSelectedChannelID(result.channel.id);
    setConversationMessages([]);
    setStreaming(null);
    void queryClient.invalidateQueries();
  }

  function clearSession() {
    clearToken();
    setSessionToken(null);
    setSelectedOrganizationID(undefined);
    setSelectedChannelID(undefined);
    setConversationMessages([]);
    setStreaming(null);
    queryClient.clear();
  }

  function handleSelectChannel(channel: Channel) {
    setSelectedChannelID(channel.id);
    setConversationMessages([]);
    setStreaming(null);
  }

  function handleMessageSent(message: Message) {
    if (
      !messageMatchesActiveConversation(message, {
        organizationID: selectedOrganizationID,
        conversationID: selectedChannelID
      })
    ) {
      return;
    }
    setConversationMessages((current) => mergeMessages(current, [message]));
  }

  if (!hasSession) {
    return <LoginView onBootstrap={handleBootstrap} />;
  }

  if (!meQuery.data) {
    return (
      <main className="loading-screen">
        <div className="loading-panel">
          <span className="product-mark">AX</span>
          <span>Loading session</span>
          <button className="text-button" type="button" onClick={clearSession}>
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
      channels={channelsQuery.data ?? []}
      selectedChannel={selectedChannel}
      messages={conversationMessages}
      messagesLoading={messagesQuery.isLoading}
      streaming={streaming}
      onSelectChannel={handleSelectChannel}
      onMessageSent={handleMessageSent}
      onLogout={clearSession}
    />
  );
}
