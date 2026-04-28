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

export interface AgentRunFailedEvent extends BaseEvent {
  type: "AgentRunFailed";
  payload: {
    run_id: string;
    agent_id?: string;
    error: string;
    team?: TeamMetadata;
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
  | AgentRunFailedEvent;

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
    event.type === "AgentRunFailed"
  );
}
