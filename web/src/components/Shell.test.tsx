// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Shell } from "./Shell";
import type { ShellProps } from "./shell/types";

const workspaceBrowserMock = vi.hoisted(() => ({
  loadTree: vi.fn(async () => undefined),
  loadFile: vi.fn(async () => undefined),
  useWorkspaceFileBrowser: vi.fn(),
}));

vi.mock("@/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
  ResizablePanel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ResizableHandle: () => <div data-testid="resize-handle" />,
}));

vi.mock("./ChannelList", () => ({
  ChannelList: ({ channels, onSelect }: any) => (
    <div data-testid="channel-list">
      {channels[0] ? (
        <button type="button" onClick={() => onSelect(channels[0])}>
          {channels[0].name}
        </button>
      ) : null}
    </div>
  ),
}));

vi.mock("./shell/AgentsSidebar", () => ({
  AgentsSidebar: ({ onCreateAgent }: { onCreateAgent: () => void }) => (
    <div data-testid="agents-sidebar">
      <button type="button" onClick={onCreateAgent}>
        Create agent
      </button>
    </div>
  ),
}));

vi.mock("./shell/ConversationPanel", () => ({
  ConversationPanel: () => <div data-testid="conversation-panel" />,
}));

vi.mock("./shell/MetricsPanel", () => ({
  MetricsPanel: () => <div data-testid="metrics-panel" />,
}));

vi.mock("./shell/TasksPanel", () => ({
  TasksPanel: () => <div data-testid="tasks-panel" />,
}));

vi.mock("./shell/AgentDetailsPanel", () => ({
  AgentDetailsPanel: () => <div data-testid="agent-details-panel" />,
}));

vi.mock("./WorkspaceFileBrowser", () => ({
  useWorkspaceFileBrowser: (args: { workspaceID?: string; workspacePath?: string }) => {
    workspaceBrowserMock.useWorkspaceFileBrowser(args);
    return {
      workspaceID: args.workspaceID,
      workspacePath: args.workspacePath,
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
      fileDownloading: false,
      fileDeleting: false,
      entryActionPending: false,
      workspaceStatus: null,
      fileOpenPosition: undefined,
      fileOpenRequestID: 0,
      fileViewMode: "edit",
      workspacePaneView: "files",
      gitEnabled: false,
      gitScope: "working_tree",
      gitTarget: "",
      gitCompare: "",
      gitStatus: undefined,
      gitStatusLoading: false,
      gitStatusError: null,
      gitDiff: undefined,
      gitDiffLoading: false,
      gitDiffError: null,
      gitSelectedPath: "",
      trimmedPath: "",
      canUseWorkspace: Boolean(args.workspaceID),
      canFetchFileBlob: Boolean(args.workspaceID),
      setFilePath: () => undefined,
      setFileBody: () => undefined,
      setFileViewMode: () => undefined,
      setWorkspacePaneView: () => undefined,
      setGitScope: () => undefined,
      setGitTarget: () => undefined,
      setGitCompare: () => undefined,
      loadTree: workspaceBrowserMock.loadTree,
      loadDirectory: async () => undefined,
      loadFile: workspaceBrowserMock.loadFile,
      loadGitStatus: async () => undefined,
      loadGitDiff: async () => undefined,
      saveFile: async () => undefined,
      fetchFileBlob: async () => new Blob(["file"]),
      downloadFile: async () => undefined,
      deleteFile: async () => undefined,
      createEntry: async () => null,
      renameEntry: async () => null,
      deleteEntry: async () => undefined,
      moveEntry: async () => null,
    };
  },
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

function setMatchMedia(matchesMaxWidth: boolean) {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query.includes("max-width") ? matchesMaxWidth : !matchesMaxWidth,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
}

beforeEach(() => {
  setMatchMedia(false);
  workspaceBrowserMock.loadTree.mockClear();
  workspaceBrowserMock.loadFile.mockClear();
  workspaceBrowserMock.useWorkspaceFileBrowser.mockClear();
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

  it("reloads the tree when switching projects while project files are open", () => {
    const props = shellProps();
    const { rerender } = render(<Shell {...props} />);

    fireEvent.click(screen.getByRole("button", { name: "Project files" }));

    expect(workspaceBrowserMock.loadTree).toHaveBeenCalledTimes(1);

    const nextProject = {
      ...props.project!,
      id: "p2",
      name: "Other",
      workspace_id: "w2",
    };
    rerender(
      <Shell
        {...props}
        projects={[props.project!, nextProject]}
        project={nextProject}
        projectWorkspace={{
          ...props.projectWorkspace!,
          id: "w2",
          name: "Other",
          path: "/workspace/Other",
        }}
      />
    );

    expect(workspaceBrowserMock.loadTree).toHaveBeenCalledTimes(2);
  });
});

describe("Shell main views", () => {
  it("returns to the conversation when the selected channel is opened from tasks", () => {
    render(<Shell {...shellProps()} />);

    fireEvent.click(screen.getAllByRole("button", { name: "Tasks" })[0]);
    expect(screen.getByTestId("tasks-panel")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "general" }));
    expect(screen.getByTestId("conversation-panel")).toBeTruthy();
    expect(screen.queryByTestId("tasks-panel")).toBeNull();
  });

  it("offers persistent runtimes when creating an agent", () => {
    render(<Shell {...shellProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

    const runtimeSelect = screen.getByRole("combobox", { name: "New agent runtime" });
    const optionValues = Array.from(runtimeSelect.querySelectorAll("option")).map((option) => option.value);

    expect(optionValues).toContain("claude-persistent");
    expect(optionValues).toContain("codex-persistent");
  });
});

describe("Shell mobile panels", () => {
  it("opens agent settings as a full-screen mobile dialog", () => {
    setMatchMedia(true);
    render(<Shell {...shellProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "Agent settings" }));

    const dialog = screen.getByTestId("mobile-agent-settings-dialog");
    expect(dialog.className).toContain("!left-0");
    expect(dialog.className).toContain("!w-[100svw]");
    expect(dialog.className).toContain("!max-w-[100svw]");
    expect(dialog.className).not.toContain("w-[96vw]");
    expect(screen.getByTestId("agent-details-panel")).toBeTruthy();
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
    onFetchWorkspaceFileBlob: vi.fn(async () => new Blob(["file"])),
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
