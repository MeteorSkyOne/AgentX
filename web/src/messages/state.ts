import type { ConversationType, Message, MessageReference } from "../api/types";
import type { AgentXEvent } from "../ws/events";

interface ActiveConversation {
  organizationID?: string;
  conversationType?: ConversationType;
  conversationID?: string;
}

export function mergeMessages(current: Message[], incoming: Message[]): Message[] {
  const messagesByID = new Map<string, Message>();
  for (const message of current) {
    messagesByID.set(message.id, message);
  }
  for (const message of incoming) {
    messagesByID.set(message.id, message);
  }

  const merged = Array.from(messagesByID.values()).sort(
    (left, right) => Date.parse(left.created_at) - Date.parse(right.created_at)
  );
  return refreshReplyReferences(merged, incoming);
}

export function removeMessageAndMarkReferencesDeleted(
  current: Message[],
  messageID: string
): Message[] {
  return current
    .filter((message) => message.id !== messageID)
    .map((message) => {
      if (message.reply_to?.message_id !== messageID) {
        return message;
      }
      return {
        ...message,
        reply_to: {
          message_id: messageID,
          deleted: true
        }
      };
    });
}

function refreshReplyReferences(messages: Message[], updatedMessages: Message[]): Message[] {
  if (updatedMessages.length === 0) {
    return messages;
  }
  const referencesByID = new Map<string, MessageReference>();
  for (const message of updatedMessages) {
    referencesByID.set(message.id, referenceFromMessage(message));
  }
  return messages.map((message) => {
    const referenceID = message.reply_to?.message_id;
    if (!referenceID) {
      return message;
    }
    const reference = referencesByID.get(referenceID);
    if (!reference) {
      return message;
    }
    return {
      ...message,
      reply_to: reference
    };
  });
}

function referenceFromMessage(message: Message): MessageReference {
  return {
    message_id: message.id,
    sender_type: message.sender_type,
    sender_id: message.sender_id,
    body: message.body,
    created_at: message.created_at
  };
}

export function eventMatchesActiveConversation(
  event: AgentXEvent,
  active: ActiveConversation
): boolean {
  if (!active.organizationID || !active.conversationID) {
    return false;
  }
  if (event.organization_id !== active.organizationID) {
    return false;
  }
  if (event.conversation_type !== active.conversationType) {
    return false;
  }
  if (event.conversation_id !== active.conversationID) {
    return false;
  }

  if (event.type === "MessageCreated" || event.type === "MessageUpdated") {
    return messageMatchesActiveConversation(event.payload.message, active);
  }

  return true;
}

export function messageMatchesActiveConversation(
  message: Message,
  active: ActiveConversation
): boolean {
  return (
    Boolean(active.organizationID) &&
    Boolean(active.conversationType) &&
    Boolean(active.conversationID) &&
    message.organization_id === active.organizationID &&
    message.conversation_type === active.conversationType &&
    message.conversation_id === active.conversationID
  );
}

export function messageHistoryLoadingForEvent(current: boolean, event: AgentXEvent): boolean {
  if (event.type === "MessageHistoryStarted") {
    return event.payload.before ? current : true;
  }
  if (event.type === "MessageHistoryCompleted") {
    return event.payload.before ? current : false;
  }
  return current;
}

interface StreamingRunState {
  agentID?: string;
  startedAt?: string;
  error?: string;
}

interface StreamingAgentContext {
  agent: {
    id: string;
    bot_user_id: string;
  };
}

export function streamingRunHasCompletedMessage(
  run: StreamingRunState,
  messages: Message[],
  agents: StreamingAgentContext[] = []
): boolean {
  if (run.error || !run.startedAt) {
    return false;
  }

  const startedAt = Date.parse(run.startedAt);
  if (!Number.isFinite(startedAt)) {
    return false;
  }

  const agent = run.agentID
    ? agents.find((item) => item.agent.id === run.agentID)?.agent
    : undefined;
  if (run.agentID && !agent) {
    return false;
  }

  return messages.some((message) => {
    if (message.sender_type !== "bot") {
      return false;
    }
    if (agent && message.sender_id !== agent.bot_user_id) {
      return false;
    }

    const messageCreatedAt = Date.parse(message.created_at);
    return Number.isFinite(messageCreatedAt) && messageCreatedAt >= startedAt;
  });
}
