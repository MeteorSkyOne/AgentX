import type { ConversationType, Message, ProcessItem, TeamMetadata } from "../api/types";

interface BaseEvent {
  id: string;
  type: string;
  organization_id: string;
  conversation_type?: ConversationType;
  conversation_id?: string;
  created_at: string;
}

export interface MessageCreatedEvent extends BaseEvent {
  type: "MessageCreated";
  payload: {
    message: Message;
  };
}

export interface MessageUpdatedEvent extends BaseEvent {
  type: "MessageUpdated";
  payload: {
    message: Message;
  };
}

export interface MessageDeletedEvent extends BaseEvent {
  type: "MessageDeleted";
  payload: {
    message_id: string;
  };
}

export interface MessageHistoryStartedEvent extends BaseEvent {
  type: "MessageHistoryStarted";
  payload: {
    before?: string;
  };
}

export interface MessageHistoryChunkEvent extends BaseEvent {
  type: "MessageHistoryChunk";
  payload: {
    messages: Message[];
  };
}

export interface MessageHistoryCompletedEvent extends BaseEvent {
  type: "MessageHistoryCompleted";
  payload: {
    has_more: boolean;
    before?: string;
  };
}

export interface AgentRunStartedEvent extends BaseEvent {
  type: "AgentRunStarted";
  payload: {
    run_id: string;
    agent_id: string;
    team?: TeamMetadata;
  };
}

export interface AgentOutputDeltaEvent extends BaseEvent {
  type: "AgentOutputDelta";
  payload: {
    run_id: string;
    agent_id?: string;
    text: string;
    thinking?: string;
    process?: ProcessItem[];
    clear_text?: boolean;
    team?: TeamMetadata;
  };
}

export interface AgentRunCompletedEvent extends BaseEvent {
  type: "AgentRunCompleted";
  payload: {
    run_id: string;
    agent_id: string;
    team?: TeamMetadata;
  };
}

export interface AgentRunCanceledEvent extends BaseEvent {
  type: "AgentRunCanceled";
  payload: {
    run_id: string;
    agent_id: string;
    team?: TeamMetadata;
  };
}

export interface AgentRunFailedEvent extends BaseEvent {
  type: "AgentRunFailed";
  payload: {
    run_id: string;
    agent_id?: string;
    error: string;
    team?: TeamMetadata;
    // When true, the failure was also stored as a chat message, so the
    // streaming placeholder can be dropped in favor of the persisted message.
    persisted?: boolean;
  };
}

export interface AgentInputRequestEvent extends BaseEvent {
  type: "AgentInputRequest";
  payload: {
    run_id: string;
    agent_id: string;
    question_id: string;
    question: string;
    options?: Array<{ label: string; description?: string }>;
    team?: TeamMetadata;
  };
}

export interface AgentPromptQueuedEvent extends BaseEvent {
  type: "AgentPromptQueued";
  payload: {
    queue_id: string;
    message_id: string;
    agent_id: string;
    body: string;
    created_at: string;
    can_steer: boolean;
  };
}

export interface AgentPromptQueueRemovedEvent extends BaseEvent {
  type: "AgentPromptQueueRemoved";
  payload: {
    queue_id: string;
    status: "started" | "steered" | "canceled" | "failed";
  };
}

export interface SubscribedEvent {
  type: "subscribed";
}

export type AgentXEvent =
  | MessageCreatedEvent
  | MessageUpdatedEvent
  | MessageDeletedEvent
  | MessageHistoryStartedEvent
  | MessageHistoryChunkEvent
  | MessageHistoryCompletedEvent
  | AgentRunStartedEvent
  | AgentOutputDeltaEvent
  | AgentRunCompletedEvent
  | AgentRunCanceledEvent
  | AgentRunFailedEvent
  | AgentInputRequestEvent
  | AgentPromptQueuedEvent
  | AgentPromptQueueRemovedEvent;

export type SocketEvent = AgentXEvent | SubscribedEvent | { type?: unknown };

export function isAgentXEvent(event: SocketEvent): event is AgentXEvent {
  return (
    event.type === "MessageCreated" ||
    event.type === "MessageUpdated" ||
    event.type === "MessageDeleted" ||
    event.type === "MessageHistoryStarted" ||
    event.type === "MessageHistoryChunk" ||
    event.type === "MessageHistoryCompleted" ||
    event.type === "AgentRunStarted" ||
    event.type === "AgentOutputDelta" ||
    event.type === "AgentRunCompleted" ||
    event.type === "AgentRunCanceled" ||
    event.type === "AgentRunFailed" ||
    event.type === "AgentInputRequest" ||
    event.type === "AgentPromptQueued" ||
    event.type === "AgentPromptQueueRemoved"
  );
}
