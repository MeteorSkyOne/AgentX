// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import type { Channel, Message } from "./api/types";
import type { AgentXEvent } from "./ws/events";

const mocks = vi.hoisted(() => ({
  socketEventHandler: undefined as ((event: AgentXEvent) => void) | undefined,
  socketConversationID: undefined as string | undefined,
  socketConversationType: undefined as string | undefined,
  loadOlderMessages: vi.fn(),
}));

vi.mock("./components/Shell", () => ({
  Shell: (props: any) => (
    <div>
      <div data-testid="message-count">{props.messages.length}</div>
      <div data-testid="message-body">{props.messages.map((message: Message) => message.body).join("\n")}</div>
      <button type="button" onClick={() => props.onSelectChannel(props.channels[0])}>
        Open channel
      </button>
    </div>
  ),
}));

vi.mock("./ws/useConversationSocket", () => ({
  useConversationSocket: vi.fn(
    (
      _organizationID: string | undefined,
      conversationType: string | undefined,
      conversationID: string | undefined,
      onEvent: (event: AgentXEvent) => void
    ) => {
      mocks.socketEventHandler = onEvent;
      mocks.socketConversationType = conversationType;
      mocks.socketConversationID = conversationID;
      return {
        connectionStatus: "connected",
        loadOlderMessages: mocks.loadOlderMessages,
      };
    }
  ),
}));

vi.mock("./api/client", () => ({
  agents: vi.fn(async () => []),
  authStatus: vi.fn(async () => ({ setup_required: false })),
  channelAgents: vi.fn(async () => []),
  channelThreads: vi.fn(async () => []),
  clearToken: vi.fn(),
  conversationContext: vi.fn(async () => ({
    project,
    channel,
    agents: [],
    binding: null,
    agent: null,
    workspace: projectWorkspace,
  })),
  createAgent: vi.fn(),
  createChannel: vi.fn(),
  createProject: vi.fn(),
  createThread: vi.fn(),
  createWorkspaceEntry: vi.fn(),
  deleteAgent: vi.fn(),
  deleteChannel: vi.fn(),
  deleteMessage: vi.fn(),
  deleteProject: vi.fn(),
  deleteThread: vi.fn(),
  deleteWorkspaceEntry: vi.fn(),
  deleteWorkspaceFile: vi.fn(),
  fetchWorkspaceFileBlob: vi.fn(async () => new Blob(["file"])),
  getToken: vi.fn(() => "session-token"),
  login: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(async () => user),
  moveWorkspaceEntry: vi.fn(),
  notificationSettings: vi.fn(async () => notificationSettingsValue),
  organizations: vi.fn(async () => [organization]),
  projectChannels: vi.fn(async () => [channel]),
  projects: vi.fn(async () => [project]),
  putWorkspaceFile: vi.fn(),
  respondToInputRequest: vi.fn(),
  serverSettings: vi.fn(async () => serverSettingsValue),
  setChannelAgents: vi.fn(),
  setupAdmin: vi.fn(),
  testNotificationSettings: vi.fn(),
  updateAgent: vi.fn(),
  updateChannel: vi.fn(),
  updateMessage: vi.fn(),
  updateNotificationSettings: vi.fn(async () => notificationSettingsValue),
  updateProject: vi.fn(),
  updateServerSettings: vi.fn(async () => serverSettingsValue),
  updateThread: vi.fn(),
  updateUserPreferences: vi.fn(async (value) => value),
  userPreferences: vi.fn(async () => ({ show_ttft: true, show_tps: true, hide_avatars: false })),
  workspace: vi.fn(async () => projectWorkspace),
  workspaceFile: vi.fn(),
  workspaceGitDiff: vi.fn(),
  workspaceGitStatus: vi.fn(),
  workspaceTree: vi.fn(),
}));

const now = "2026-05-01T00:00:00Z";
const user = { id: "u1", display_name: "Meteorsky", created_at: now };
const organization = { id: "org1", name: "AgentX", created_at: now };
const project = {
  id: "prj1",
  organization_id: organization.id,
  name: "AgentX",
  workspace_id: "wks1",
  created_by: user.id,
  created_at: now,
  updated_at: now,
};
const projectWorkspace = {
  id: "wks1",
  organization_id: organization.id,
  type: "project",
  name: "AgentX",
  path: "/tmp/agentx",
  created_by: user.id,
  created_at: now,
  updated_at: now,
};
const channel: Channel = {
  id: "chn1",
  organization_id: organization.id,
  project_id: project.id,
  type: "text",
  name: "general",
  team_max_batches: 6,
  team_max_runs: 12,
  created_at: now,
  updated_at: now,
};
const notificationSettingsValue = {
  organization_id: organization.id,
  webhook_enabled: false,
  webhook_url: "",
  webhook_secret_configured: false,
  created_at: now,
  updated_at: now,
};
const serverSettingsValue = {
  organization_id: organization.id,
  listen_ip: "127.0.0.1",
  listen_port: 8080,
  addr_override_active: false,
  effective_addr: "127.0.0.1:8080",
  effective_http_addr: "127.0.0.1:8080",
  restart_required: false,
  tls: {
    enabled: false,
    listen_port: 8443,
    cert_file: "",
    key_file: "",
  },
};

beforeEach(() => {
  mocks.socketEventHandler = undefined;
  mocks.socketConversationID = undefined;
  mocks.socketConversationType = undefined;
  mocks.loadOlderMessages.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("App conversation selection", () => {
  it("keeps messages when the current channel is selected again", async () => {
    renderWithClient(<App />);

    await screen.findByTestId("message-count");
    await waitFor(() => {
      expect(mocks.socketConversationType).toBe("channel");
      expect(mocks.socketConversationID).toBe(channel.id);
      expect(mocks.socketEventHandler).toBeTruthy();
    });

    act(() => {
      mocks.socketEventHandler?.(messageHistoryChunk([message("msg1", "keep me")]));
    });

    expect(screen.getByTestId("message-count").textContent).toBe("1");
    expect(screen.getByTestId("message-body").textContent).toBe("keep me");

    fireEvent.click(screen.getByRole("button", { name: "Open channel" }));

    expect(screen.getByTestId("message-count").textContent).toBe("1");
    expect(screen.getByTestId("message-body").textContent).toBe("keep me");
  });
});

function renderWithClient(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
}

function message(id: string, body: string): Message {
  return {
    id,
    organization_id: organization.id,
    conversation_type: "channel",
    conversation_id: channel.id,
    sender_type: "user",
    sender_id: user.id,
    kind: "text",
    body,
    created_at: now,
  };
}

function messageHistoryChunk(messages: Message[]): AgentXEvent {
  return {
    id: "evt1",
    type: "MessageHistoryChunk",
    organization_id: organization.id,
    conversation_type: "channel",
    conversation_id: channel.id,
    created_at: now,
    payload: { messages },
  };
}
