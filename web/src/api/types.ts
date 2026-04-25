export type ConversationType = "channel" | "thread" | "dm";
export type SenderType = "user" | "bot" | "system";
export type MessageKind = "text" | "event";

export interface User {
  id: string;
  display_name: string;
  created_at: string;
}

export interface Organization {
  id: string;
  name: string;
  created_at: string;
}

export interface Channel {
  id: string;
  organization_id: string;
  name: string;
  created_at: string;
}

export interface BotUser {
  id: string;
  organization_id: string;
  display_name: string;
  created_at: string;
}

export interface Agent {
  id: string;
  organization_id: string;
  bot_user_id: string;
  kind: string;
  name: string;
  model: string;
  default_workspace_id: string;
  created_at: string;
  updated_at: string;
}

export interface Workspace {
  id: string;
  organization_id: string;
  type: string;
  name: string;
  path: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface Message {
  id: string;
  organization_id: string;
  conversation_type: ConversationType;
  conversation_id: string;
  sender_type: SenderType;
  sender_id: string;
  kind: MessageKind;
  body: string;
  created_at: string;
}

export interface BootstrapResponse {
  session_token: string;
  user: User;
  organization: Organization;
  channel: Channel;
  bot_user: BotUser;
  agent: Agent;
  workspace: Workspace;
}
