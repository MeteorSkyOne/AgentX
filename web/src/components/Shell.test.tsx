// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Shell } from "./Shell";
import type { ShellProps } from "./shell/types";

vi.mock("@/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
  ResizablePanel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ResizableHandle: () => <div data-testid="resize-handle" />,
}));

vi.mock("./ChannelList", () => ({
  ChannelList: () => <div data-testid="channel-list" />,
}));

vi.mock("./shell/AgentsSidebar", () => ({
  AgentsSidebar: () => <div data-testid="agents-sidebar" />,
}));

vi.mock("./shell/ConversationPanel", () => ({
  ConversationPanel: () => <div data-testid="conversation-panel" />,
}));

vi.mock("./shell/MetricsPanel", () => ({
  MetricsPanel: () => <div data-testid="metrics-panel" />,
}));

vi.mock("./WorkspaceFileBrowser", () => ({
  useWorkspaceFileBrowser: () => ({
    workspaceID: "w1",
    workspacePath: "/workspace/AgentX",
    filePath: "",
    fileBody: "",
    tree: undefined,
    workspaceTreeResetKey: 0,
    workspaceTreeLoading: false,
    workspaceTreeError: null,
    directoryLoadingPaths: new Set(),
    directoryLoadErrors: {},
    fileLoading: false,
    fileLoadError: null,
    fileSaving: false,
    fileDeleting: false,
    entryActionPending: false,
    workspaceStatus: null,
    fileOpenPosition: undefined,
    fileOpenRequestID: 0,
    fileViewMode: "edit",
    trimmedPath: "",
    canUseWorkspace: true,
    setFilePath: () => undefined,
    setFileBody: () => undefined,
    setFileViewMode: () => undefined,
    loadTree: async () => undefined,
    loadDirectory: async () => undefined,
    loadFile: async () => undefined,
    saveFile: async () => undefined,
    deleteFile: async () => undefined,
    createEntry: async () => null,
    renameEntry: async () => null,
    deleteEntry: async () => undefined,
    moveEntry: async () => null,
  }),
  WorkspaceFileTreePane: ({ toolbarEnd }: { toolbarEnd?: ReactNode }) => (
    <div data-testid="project-file-tree-pane">{toolbarEnd}</div>
  ),
  WorkspaceFileEditorPane: ({
    toolbarEnd,
    editorClassName,
  }: {
    toolbarEnd?: ReactNode;
    editorClassName?: string;
  }) => (
    <div data-editor-class-name={editorClassName} data-testid="project-file-editor-pane">{toolbarEnd}</div>
  ),
}));

beforeEach(() => {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query.includes("max-width") ? false : true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

afterEach(() => {
  cleanup();
});

describe("Shell project files overlay", () => {
  it("keeps the conversation mounted while project files are open", () => {
    render(<Shell {...shellProps()} />);

    expect(screen.getByTestId("conversation-panel")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Project files" }));

    expect(screen.getByTestId("project-files-overlay")).toBeTruthy();
    expect(screen.getByTestId("conversation-panel")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Close project files" }));

    expect(screen.queryByTestId("project-files-overlay")).toBeNull();
    expect(screen.getByTestId("conversation-panel")).toBeTruthy();
  });

  it("collapses the desktop project file tree panel", () => {
    render(<Shell {...shellProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "Project files" }));

    expect(screen.getByTestId("project-file-tree-pane")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Hide project file tree" }));

    expect(screen.queryByTestId("project-file-tree-pane")).toBeNull();
    expect(screen.getByRole("button", { name: "Show project file tree" })).toBeTruthy();
    expect(screen.getByTestId("project-file-editor-pane").getAttribute("data-editor-class-name")).toBe("md:mx-6");

    fireEvent.click(screen.getByRole("button", { name: "Show project file tree" }));

    expect(screen.getByTestId("project-file-tree-pane")).toBeTruthy();
  });
});

function shellProps(): ShellProps {
  const project = {
    id: "p1",
    organization_id: "o1",
    name: "AgentX",
    workspace_id: "w1",
    created_by: "u1",
    created_at: "2026-04-28T00:00:00Z",
    updated_at: "2026-04-28T00:00:00Z",
  };
  const channel = {
    id: "c1",
    organization_id: "o1",
    project_id: "p1",
    type: "text" as const,
    name: "general",
    team_max_batches: 1,
    team_max_runs: 1,
    created_at: "2026-04-28T00:00:00Z",
    updated_at: "2026-04-28T00:00:00Z",
  };

  return {
    user: {
      id: "u1",
      display_name: "Meteorsky",
      created_at: "2026-04-28T00:00:00Z",
    },
    organization: {
      id: "o1",
      name: "AgentX",
      created_at: "2026-04-28T00:00:00Z",
    },
    projects: [project],
    project,
    projectWorkspace: {
      id: "w1",
      organization_id: "o1",
      type: "project",
      name: "AgentX",
      path: "/workspace/AgentX",
      created_by: "u1",
      created_at: "2026-04-28T00:00:00Z",
      updated_at: "2026-04-28T00:00:00Z",
    },
    channels: [channel],
    selectedChannel: channel,
    activeConversation: {
      type: "channel",
      id: "c1",
      projectID: "p1",
      channelID: "c1",
    },
    threads: [],
    agents: [],
    channelAgents: [],
    contextLoading: false,
    messages: [],
    messagesLoading: false,
    olderMessagesLoading: false,
    hasOlderMessages: false,
    streaming: [],
    connectionStatus: "connected",
    notificationSettingsLoading: false,
    serverSettingsLoading: false,
    serverSettingsError: null,
    preferences: { show_ttft: true, show_tps: true, hide_avatars: false },
    preferencesLoading: false,
    theme: "dark",
    onSelectProject: vi.fn(),
    onCreateProject: vi.fn(),
    onUpdateProject: vi.fn(),
    onDeleteProject: vi.fn(),
    onSelectChannel: vi.fn(),
    onCreateChannel: vi.fn(),
    onUpdateChannel: vi.fn(),
    onDeleteChannel: vi.fn(),
    onSelectThread: vi.fn(),
    onCreateThread: vi.fn(),
    onUpdateThread: vi.fn(),
    onDeleteThread: vi.fn(),
    onSaveChannelAgents: vi.fn(),
    onCreateAgent: vi.fn(),
    onUpdateAgent: vi.fn(),
    onDeleteAgent: vi.fn(),
    onUpdateNotificationSettings: vi.fn(),
    onUpdateServerSettings: vi.fn(),
    onUpdateUserPreferences: vi.fn(),
    onTestNotificationSettings: vi.fn(),
    onLoadWorkspaceTree: vi.fn(),
    onReadWorkspaceFile: vi.fn(),
    onWriteWorkspaceFile: vi.fn(),
    onDeleteWorkspaceFile: vi.fn(),
    onCreateWorkspaceEntry: vi.fn(),
    onMoveWorkspaceEntry: vi.fn(),
    onDeleteWorkspaceEntry: vi.fn(),
    onLoadWorkspaceGitStatus: vi.fn(),
    onLoadWorkspaceGitDiff: vi.fn(),
    onUpdateMessage: vi.fn(),
    onDeleteMessage: vi.fn(),
    onLoadOlderMessages: vi.fn(),
    onMessageSent: vi.fn(),
    onToggleTheme: vi.fn(),
    onLogout: vi.fn(),
  };
}
