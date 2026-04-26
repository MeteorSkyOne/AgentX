import type { ConversationType, Message, ProcessItem } from "../api/types";

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

export interface AgentRunStartedEvent extends BaseEvent {
  type: "AgentRunStarted";
  payload: {
    run_id: string;
    agent_id: string;
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
  };
}

export interface AgentRunCompletedEvent extends BaseEvent {
  type: "AgentRunCompleted";
  payload: {
    run_id: string;
    agent_id: string;
  };
}

export interface AgentRunFailedEvent extends BaseEvent {
  type: "AgentRunFailed";
  payload: {
    run_id: string;
    error: string;
  };
}

export interface SubscribedEvent {
  type: "subscribed";
}

export type AgentXEvent =
  | MessageCreatedEvent
  | MessageUpdatedEvent
  | MessageDeletedEvent
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
    event.type === "AgentRunStarted" ||
    event.type === "AgentOutputDelta" ||
    event.type === "AgentRunCompleted" ||
    event.type === "AgentRunFailed"
  );
}
