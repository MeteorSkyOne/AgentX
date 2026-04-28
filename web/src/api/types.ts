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

export interface MessageMetricsSummary {
  run_id: string;
  provider: string;
  ttft_ms?: number | null;
  tps?: number | null;
  duration_ms?: number | null;
  input_tokens?: number | null;
  output_tokens?: number | null;
  total_tokens?: number | null;
  cache_hit_rate?: number | null;
}

export interface TeamMetadata {
  session_id: string;
  root_message_id: string;
  leader_agent_id: string;
  phase: "leader" | "discussion" | "summary" | string;
  turn: number;
  source_message_id?: string;
}

export interface MessageMetadata {
  thinking?: string;
  process?: ProcessItem[];
  metrics?: MessageMetricsSummary;
  team?: TeamMetadata;
  [key: string]: JsonValue | ProcessItem[] | MessageMetricsSummary | TeamMetadata | undefined;
}

export interface MessageReference {
  message_id: string;
  deleted?: boolean;
  sender_type?: SenderType;
  sender_id?: string;
  body?: string;
  attachment_count?: number;
  created_at?: string;
}

export interface User {
  id: string;
  username?: string;
  display_name: string;
  created_at: string;
}

export interface Organization {
  id: string;
  name: string;
  created_at: string;
}

export interface NotificationSettings {
  organization_id: string;
  webhook_enabled: boolean;
  webhook_url: string;
  webhook_secret_configured: boolean;
  created_at: string;
  updated_at: string;
}

export interface UserPreferences {
  show_ttft: boolean;
  show_tps: boolean;
}

export type MetricsProvider = "claude" | "codex" | "fake";

export interface AgentRunMetric {
  run_id: string;
  organization_id: string;
  project_id?: string;
  project_name?: string;
  channel_id?: string;
  channel_name?: string;
  thread_id?: string;
  thread_title?: string;
  conversation_type: ConversationType;
  conversation_id: string;
  message_id: string;
  response_message_id?: string;
  agent_id: string;
  agent_name: string;
  provider: string;
  model: string;
  status: string;
  run_count?: number;
  completed_runs?: number;
  failed_runs?: number;
  started_at: string;
  first_token_at?: string | null;
  completed_at?: string | null;
  ttft_ms?: number | null;
  duration_ms?: number | null;
  tps?: number | null;
  input_tokens?: number | null;
  cached_input_tokens?: number | null;
  cache_creation_input_tokens?: number | null;
  cache_read_input_tokens?: number | null;
  output_tokens?: number | null;
  reasoning_output_tokens?: number | null;
  total_tokens?: number | null;
  cache_hit_rate?: number | null;
  total_cost_usd?: number | null;
  created_at: string;
}

export interface Channel {
  id: string;
  organization_id: string;
  project_id: string;
  type: "text" | "thread";
  name: string;
  team_max_batches: number;
  team_max_runs: number;
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
  description: string;
  model: string;
  effort: string;
  config_workspace_id: string;
  default_workspace_id: string;
  enabled: boolean;
  fast_mode: boolean;
  yolo_mode: boolean;
  env?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export type AgentProviderLimitStatus = "ok" | "unavailable" | "error";

export interface AgentProviderLimitAuth {
  logged_in: boolean;
  method?: string;
  provider?: string;
  plan?: string;
}

export interface AgentProviderLimitWindow {
  kind: string;
  label: string;
  used_percent: number | null;
  window_minutes: number;
  resets_at: string | null;
}

export interface AgentProviderLimits {
  agent_id: string;
  provider: string;
  status: AgentProviderLimitStatus;
  auth: AgentProviderLimitAuth;
  windows: AgentProviderLimitWindow[];
  fetched_at: string;
  cache_ttl_seconds: number;
  message?: string;
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

export interface AgentChannelContext {
  binding: ChannelAgent;
  channel: Channel;
  project: Project;
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
  reply_to_message_id?: string;
  reply_to?: MessageReference;
  attachments?: MessageAttachment[];
  created_at: string;
}

export interface MessageAttachment {
  id: string;
  message_id: string;
  filename: string;
  content_type: string;
  kind: "image" | "text";
  size_bytes: number;
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

export interface AuthStatus {
  setup_required: boolean;
  setup_token_required: boolean;
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
