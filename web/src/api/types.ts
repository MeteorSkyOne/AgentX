export type ConversationType = "channel" | "thread" | "dm";
export type SenderType = "user" | "bot" | "system";
export type MessageKind = "text" | "event";
export type JsonValue =
  | null
  | string
  | number
  | boolean
  | JsonValue[]
  | { [key: string]: JsonValue };

export interface ProcessItem {
  type: "thinking" | "tool_call" | "tool_result";
  text?: string;
  tool_name?: string;
  tool_call_id?: string;
  status?: string;
  input?: JsonValue;
  output?: JsonValue;
  raw?: JsonValue;
  created_at?: string;
}

export interface MessageMetadata {
  thinking?: string;
  process?: ProcessItem[];
  [key: string]: JsonValue | ProcessItem[] | undefined;
}

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
  project_id: string;
  type: "text" | "thread";
  name: string;
  created_at: string;
  updated_at: string;
  archived_at?: string;
}

export interface Project {
  id: string;
  organization_id: string;
  name: string;
  workspace_id: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface Thread {
  id: string;
  organization_id: string;
  project_id: string;
  channel_id: string;
  title: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  archived_at?: string;
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
  handle: string;
  model: string;
  config_workspace_id: string;
  default_workspace_id: string;
  enabled: boolean;
  yolo_mode: boolean;
  env?: Record<string, string>;
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

export interface ConversationBinding {
  id: string;
  organization_id: string;
  conversation_type: ConversationType;
  conversation_id: string;
  agent_id: string;
  workspace_id: string;
  created_at: string;
  updated_at: string;
}

export interface ChannelAgent {
  channel_id: string;
  agent_id: string;
  run_workspace_id?: string;
  created_at: string;
  updated_at: string;
}

export interface ConversationAgentContext {
  binding: ChannelAgent;
  agent: Agent;
  config_workspace: Workspace;
  run_workspace: Workspace;
}

export interface ConversationContext {
  project: Project;
  channel: Channel;
  thread?: Thread;
  agents: ConversationAgentContext[];
  binding: ConversationBinding;
  agent: Agent;
  workspace: Workspace;
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
  metadata?: MessageMetadata;
  created_at: string;
}

export interface BootstrapResponse {
  session_token: string;
  user: User;
  organization: Organization;
  project: Project;
  channel: Channel;
  bot_user: BotUser;
  agent: Agent;
  workspace: Workspace;
  project_workspace: Workspace;
}

export interface AuthResponse {
  session_token: string;
  user: User;
}

export interface CreateThreadResponse {
  thread: Thread;
  message: Message;
}

export interface WorkspaceTreeEntry {
  name: string;
  path: string;
  type: "directory" | "file";
  children?: WorkspaceTreeEntry[];
}

export interface WorkspaceFile {
  path: string;
  body: string;
  content: string;
}
