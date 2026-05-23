import type { Agent, ConversationAgentContext, ConversationType, Message, ProcessItem, TeamMetadata } from "@/api/types";
import type { QueuedPrompt } from "@/components/shell/types";

export interface ActiveConversation {
  type: ConversationType;
  id: string;
  projectID: string;
  channelID: string;
}

export interface PageSelection {
  organizationID?: string;
  projectID?: string;
  channelID?: string;
  conversationType?: ConversationType;
  conversationID?: string;
}

export interface StreamingMessage {
  runID: string;
  agentID?: string;
  startedAt?: string;
  endedAt?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
  team?: TeamMetadata;
}

const PAGE_SELECTION_STORAGE_KEY = "agentx:last-selection";

export function isConversationType(value: unknown): value is ConversationType {
  return value === "channel" || value === "thread" || value === "dm";
}

export function getStoredPageSelection(): PageSelection {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const raw = window.localStorage.getItem(PAGE_SELECTION_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw);
    return {
      organizationID: typeof parsed.organizationID === "string" ? parsed.organizationID : undefined,
      projectID: typeof parsed.projectID === "string" ? parsed.projectID : undefined,
      channelID: typeof parsed.channelID === "string" ? parsed.channelID : undefined,
      conversationType: isConversationType(parsed.conversationType) ? parsed.conversationType : undefined,
      conversationID: typeof parsed.conversationID === "string" ? parsed.conversationID : undefined
    };
  } catch {
    return {};
  }
}

export function storePageSelection(selection: PageSelection) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(PAGE_SELECTION_STORAGE_KEY, JSON.stringify(selection));
  } catch {
    // Ignore storage errors so private browsing or quota issues do not break the app.
  }
}

export function clearPageSelection() {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.removeItem(PAGE_SELECTION_STORAGE_KEY);
  } catch {
    // Ignore storage errors so session cleanup can continue.
  }
}

export function activeConversationFromSelection(selection: PageSelection): ActiveConversation | undefined {
  if (!selection.projectID || !selection.channelID || !selection.conversationType || !selection.conversationID) {
    return undefined;
  }
  if (selection.conversationType === "channel" && selection.conversationID !== selection.channelID) {
    return undefined;
  }
  if (selection.conversationType === "dm") {
    return undefined;
  }
  return {
    type: selection.conversationType,
    id: selection.conversationID,
    projectID: selection.projectID,
    channelID: selection.channelID
  };
}

export function conversationKey(conversation?: ActiveConversation): string {
  return conversation ? `${conversation.type}:${conversation.id}` : "";
}

export function isSystemCommandMessage(message: Message): boolean {
  return message.sender_type === "system" && message.metadata?.command === true;
}

export function agentNameForID(
  agentID: string,
  conversationAgents?: ConversationAgentContext[],
  allAgents?: Agent[]
): string | undefined {
  return (
    conversationAgents?.find((item) => item.agent.id === agentID)?.agent.name ??
    allAgents?.find((agent) => agent.id === agentID)?.name
  );
}

export function upsertQueuedPrompt(current: QueuedPrompt[], item: QueuedPrompt): QueuedPrompt[] {
  const next = current.filter((queued) => queued.queueID !== item.queueID);
  next.push(item);
  return next.sort((left, right) => Date.parse(left.createdAt) - Date.parse(right.createdAt));
}
