import type {
  AgentRunMetric,
  AuthResponse,
  AuthStatus,
  Agent,
  AgentChannelContext,
  Channel,
  ConversationAgentContext,
  ConversationContext,
  ConversationType,
  CreateThreadResponse,
  Message,
  MetricsProvider,
  NotificationSettings,
  Organization,
  Project,
  Thread,
  User,
  UserPreferences,
  Workspace,
  WorkspaceFile,
  WorkspaceTreeEntry
} from "./types";

const tokenKey = "agentx.session_token";

export function getToken(): string | null {
  return localStorage.getItem(tokenKey);
}

export function setToken(token: string): void {
  localStorage.setItem(tokenKey, token);
}

export function clearToken(): void {
  localStorage.removeItem(tokenKey);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers = new Headers(init.headers);

  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(path, {
    ...init,
    headers
  });

  if (!response.ok) {
    const message = await errorMessage(response);
    throw new Error(message);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}

async function errorMessage(response: Response): Promise<string> {
  const fallback = `${response.status} ${response.statusText}`.trim();
  try {
    const body = (await response.json()) as { error?: string; message?: string };
    return body.error ?? body.message ?? fallback;
  } catch {
    return fallback;
  }
}

export function authStatus(): Promise<AuthStatus> {
  return request<AuthStatus>("/api/auth/status");
}

export function setupAdmin(payload: {
  setup_token: string;
  username: string;
  password: string;
  display_name: string;
}): Promise<AuthResponse> {
  return request<AuthResponse>("/api/auth/setup", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function login(username: string, password: string): Promise<AuthResponse> {
  return request<AuthResponse>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({
      username,
      password
    })
  });
}

export function logout(): Promise<void> {
  return request<void>("/api/auth/logout", {
    method: "POST"
  });
}

export function me(): Promise<User> {
  return request<User>("/api/me");
}

export function userPreferences(): Promise<UserPreferences> {
  return request<UserPreferences>("/api/me/preferences");
}

export function updateUserPreferences(payload: UserPreferences): Promise<UserPreferences> {
  return request<UserPreferences>("/api/me/preferences", {
    method: "PUT",
    body: JSON.stringify(payload)
  });
}

export function organizations(): Promise<Organization[]> {
  return request<Organization[]>("/api/organizations");
}

export function notificationSettings(orgID: string): Promise<NotificationSettings> {
  return request<NotificationSettings>(
    `/api/organizations/${encodeURIComponent(orgID)}/notification-settings`
  );
}

export function updateNotificationSettings(
  orgID: string,
  payload: {
    webhook_enabled: boolean;
    webhook_url: string;
    webhook_secret?: string;
  }
): Promise<NotificationSettings> {
  return request<NotificationSettings>(
    `/api/organizations/${encodeURIComponent(orgID)}/notification-settings`,
    {
      method: "PUT",
      body: JSON.stringify(payload)
    }
  );
}

export function testNotificationSettings(orgID: string): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>(
    `/api/organizations/${encodeURIComponent(orgID)}/notification-settings/test`,
    {
      method: "POST"
    }
  );
}

export function channels(orgID: string): Promise<Channel[]> {
  return request<Channel[]>(`/api/organizations/${encodeURIComponent(orgID)}/channels`);
}

export function projects(orgID: string): Promise<Project[]> {
  return request<Project[]>(`/api/organizations/${encodeURIComponent(orgID)}/projects`);
}

export function createProject(orgID: string, name: string): Promise<Project> {
  return request<Project>(`/api/organizations/${encodeURIComponent(orgID)}/projects`, {
    method: "POST",
    body: JSON.stringify({ name })
  });
}

export function updateProject(
  projectID: string,
  payload: { name?: string; workspace_path?: string }
): Promise<Project> {
  return request<Project>(`/api/projects/${encodeURIComponent(projectID)}`, {
    method: "PATCH",
    body: JSON.stringify(payload)
  });
}

export function deleteProject(projectID: string): Promise<void> {
  return request<void>(`/api/projects/${encodeURIComponent(projectID)}`, {
    method: "DELETE"
  });
}

export function workspace(workspaceID: string): Promise<Workspace> {
  return request<Workspace>(`/api/workspaces/${encodeURIComponent(workspaceID)}`);
}

export function projectChannels(projectID: string): Promise<Channel[]> {
  return request<Channel[]>(`/api/projects/${encodeURIComponent(projectID)}/channels`);
}

export function projectMetrics(
  projectID: string,
  options: MetricsOptions = {}
): Promise<AgentRunMetric[]> {
  const params = metricsParams(options);
  return request<AgentRunMetric[]>(
    `/api/projects/${encodeURIComponent(projectID)}/metrics${params}`
  );
}

export function createChannel(
  projectID: string,
  name: string,
  type: Channel["type"]
): Promise<Channel> {
  return request<Channel>(`/api/projects/${encodeURIComponent(projectID)}/channels`, {
    method: "POST",
    body: JSON.stringify({ name, type })
  });
}

export function updateChannel(
  channelID: string,
  payload: { name: string; type?: Channel["type"] }
): Promise<Channel> {
  return request<Channel>(`/api/channels/${encodeURIComponent(channelID)}`, {
    method: "PATCH",
    body: JSON.stringify(payload)
  });
}

export function deleteChannel(channelID: string): Promise<void> {
  return request<void>(`/api/channels/${encodeURIComponent(channelID)}`, {
    method: "DELETE"
  });
}

export function channelThreads(channelID: string): Promise<Thread[]> {
  return request<Thread[]>(`/api/channels/${encodeURIComponent(channelID)}/threads`);
}

export function channelMetrics(
  channelID: string,
  options: MetricsOptions = {}
): Promise<AgentRunMetric[]> {
  const params = metricsParams(options);
  return request<AgentRunMetric[]>(
    `/api/channels/${encodeURIComponent(channelID)}/metrics${params}`
  );
}

export function createThread(
  channelID: string,
  title: string,
  body: string
): Promise<CreateThreadResponse> {
  return request<CreateThreadResponse>(`/api/channels/${encodeURIComponent(channelID)}/threads`, {
    method: "POST",
    body: JSON.stringify({ title, body })
  });
}

export function updateThread(threadID: string, title: string): Promise<Thread> {
  return request<Thread>(`/api/threads/${encodeURIComponent(threadID)}`, {
    method: "PATCH",
    body: JSON.stringify({ title })
  });
}

export function deleteThread(threadID: string): Promise<void> {
  return request<void>(`/api/threads/${encodeURIComponent(threadID)}`, {
    method: "DELETE"
  });
}

export function agents(orgID: string): Promise<Agent[]> {
  return request<Agent[]>(`/api/organizations/${encodeURIComponent(orgID)}/agents`);
}

export function createAgent(
  orgID: string,
  payload: {
    name: string;
    description?: string;
    handle?: string;
    kind?: string;
    model?: string;
    effort?: string;
    fast_mode?: boolean;
    yolo_mode?: boolean;
    env?: Record<string, string>;
  }
): Promise<Agent> {
  return request<Agent>(`/api/organizations/${encodeURIComponent(orgID)}/agents`, {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function updateAgent(
  agentID: string,
  payload: Partial<Pick<Agent, "name" | "description" | "handle" | "kind" | "model" | "effort" | "enabled" | "fast_mode" | "yolo_mode">> & {
    env?: Record<string, string>;
  }
): Promise<Agent> {
  return request<Agent>(`/api/agents/${encodeURIComponent(agentID)}`, {
    method: "PATCH",
    body: JSON.stringify(payload)
  });
}

export function deleteAgent(agentID: string): Promise<void> {
  return request<void>(`/api/agents/${encodeURIComponent(agentID)}`, {
    method: "DELETE"
  });
}

export function agentChannels(agentID: string): Promise<AgentChannelContext[]> {
  return request<AgentChannelContext[]>(`/api/agents/${encodeURIComponent(agentID)}/channels`);
}

export function channelAgents(channelID: string): Promise<ConversationAgentContext[]> {
  return request<ConversationAgentContext[]>(
    `/api/channels/${encodeURIComponent(channelID)}/agents`
  );
}

export function setChannelAgents(
  channelID: string,
  bindings: Array<{ agent_id: string; run_workspace_id?: string }>
): Promise<ConversationAgentContext[]> {
  return request<ConversationAgentContext[]>(
    `/api/channels/${encodeURIComponent(channelID)}/agents`,
    {
      method: "PUT",
      body: JSON.stringify({ agents: bindings })
    }
  );
}

export function workspaceTree(workspaceID: string): Promise<WorkspaceTreeEntry> {
  return request<WorkspaceTreeEntry>(`/api/workspaces/${encodeURIComponent(workspaceID)}/tree`);
}

export function workspaceFile(workspaceID: string, path: string): Promise<WorkspaceFile> {
  const params = new URLSearchParams({ path });
  return request<WorkspaceFile>(
    `/api/workspaces/${encodeURIComponent(workspaceID)}/files?${params.toString()}`
  );
}

export function putWorkspaceFile(
  workspaceID: string,
  path: string,
  body: string
): Promise<WorkspaceFile> {
  const params = new URLSearchParams({ path });
  return request<WorkspaceFile>(
    `/api/workspaces/${encodeURIComponent(workspaceID)}/files?${params.toString()}`,
    {
      method: "PUT",
      body: JSON.stringify({ body })
    }
  );
}

export function deleteWorkspaceFile(workspaceID: string, path: string): Promise<void> {
  const params = new URLSearchParams({ path });
  return request<void>(
    `/api/workspaces/${encodeURIComponent(workspaceID)}/files?${params.toString()}`,
    {
      method: "DELETE"
    }
  );
}

export function messages(type: ConversationType, id: string): Promise<Message[]> {
  return request<Message[]>(
    `/api/conversations/${encodeURIComponent(type)}/${encodeURIComponent(id)}/messages`
  );
}

export function conversationContext(
  type: ConversationType,
  id: string
): Promise<ConversationContext> {
  return request<ConversationContext>(
    `/api/conversations/${encodeURIComponent(type)}/${encodeURIComponent(id)}/context`
  );
}

export function conversationMetrics(
  type: ConversationType,
  id: string,
  options: MetricsOptions = {}
): Promise<AgentRunMetric[]> {
  const params = metricsParams(options);
  return request<AgentRunMetric[]>(
    `/api/conversations/${encodeURIComponent(type)}/${encodeURIComponent(id)}/metrics${params}`
  );
}

export function sendMessage(
  type: ConversationType,
  id: string,
  body: string,
  options: { replyToMessageID?: string } = {}
): Promise<Message> {
  const payload: { body: string; reply_to_message_id?: string } = { body };
  if (options.replyToMessageID) {
    payload.reply_to_message_id = options.replyToMessageID;
  }
  return request<Message>(
    `/api/conversations/${encodeURIComponent(type)}/${encodeURIComponent(id)}/messages`,
    {
      method: "POST",
      body: JSON.stringify(payload)
    }
  );
}

export function updateMessage(messageID: string, body: string): Promise<Message> {
  return request<Message>(`/api/messages/${encodeURIComponent(messageID)}`, {
    method: "PATCH",
    body: JSON.stringify({ body })
  });
}

export function deleteMessage(messageID: string): Promise<void> {
  return request<void>(`/api/messages/${encodeURIComponent(messageID)}`, {
    method: "DELETE"
  });
}

type MetricsOptions = {
  limit?: number;
  provider?: MetricsProvider | "";
  group?: "agent" | "";
};

function metricsParams(options: MetricsOptions): string {
  const params = new URLSearchParams();
  if (options.limit) {
    params.set("limit", String(options.limit));
  }
  if (options.provider) {
    params.set("provider", options.provider);
  }
  if (options.group) {
    params.set("group", options.group);
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}
