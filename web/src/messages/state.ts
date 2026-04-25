import type { Message } from "../api/types";
import type { AgentXEvent } from "../ws/events";

interface ActiveConversation {
  organizationID?: string;
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

  return Array.from(messagesByID.values()).sort(
    (left, right) => Date.parse(left.created_at) - Date.parse(right.created_at)
  );
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
  if (event.conversation_type && event.conversation_type !== "channel") {
    return false;
  }
  if (event.conversation_id !== active.conversationID) {
    return false;
  }

  if (event.type === "MessageCreated") {
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
    Boolean(active.conversationID) &&
    message.organization_id === active.organizationID &&
    message.conversation_type === "channel" &&
    message.conversation_id === active.conversationID
  );
}
