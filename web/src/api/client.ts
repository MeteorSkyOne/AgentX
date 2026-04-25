import type {
  BootstrapResponse,
  Channel,
  ConversationType,
  Message,
  Organization,
  User
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

export function bootstrap(adminToken: string, displayName: string): Promise<BootstrapResponse> {
  return request<BootstrapResponse>("/api/auth/bootstrap", {
    method: "POST",
    body: JSON.stringify({
      admin_token: adminToken,
      display_name: displayName
    })
  });
}

export function me(): Promise<User> {
  return request<User>("/api/me");
}

export function organizations(): Promise<Organization[]> {
  return request<Organization[]>("/api/organizations");
}

export function channels(orgID: string): Promise<Channel[]> {
  return request<Channel[]>(`/api/organizations/${encodeURIComponent(orgID)}/channels`);
}

export function messages(type: ConversationType, id: string): Promise<Message[]> {
  return request<Message[]>(
    `/api/conversations/${encodeURIComponent(type)}/${encodeURIComponent(id)}/messages`
  );
}

export function sendMessage(
  type: ConversationType,
  id: string,
  body: string
): Promise<Message> {
  return request<Message>(
    `/api/conversations/${encodeURIComponent(type)}/${encodeURIComponent(id)}/messages`,
    {
      method: "POST",
      body: JSON.stringify({ body })
    }
  );
}
