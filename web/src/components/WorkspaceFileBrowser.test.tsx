// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceGitHistory } from "@/api/types";
import {
  WorkspaceFileEditorPane,
  WorkspaceFileTreePane,
  useWorkspaceFileBrowser,
  type WorkspaceFileBrowserController,
  type WorkspaceFileViewMode,
} from "./WorkspaceFileBrowser";

vi.mock("./WorkspaceFileEditor", () => ({
  WorkspaceFileEditor: ({
    controller,
    className,
  }: {
    controller: WorkspaceFileBrowserController;
    className?: string;
  }) => <div className={className} data-testid="mock-workspace-file-editor">{controller.fileViewMode}</div>,
  WorkspaceGitDiffViewer: () => <div data-testid="mock-workspace-git-diff-viewer" />,
}));

afterEach(() => {
  vi.useRealTimers();
  cleanup();
});

describe("WorkspaceFileEditorPane markdown controls", () => {
  it("can defer tree loading until explicitly requested", async () => {
    const onLoadTree = vi.fn(async () => workspaceTreeFixture());

    render(<WorkspaceFileBrowserHookHarness autoLoadTree={false} onLoadTree={onLoadTree} />);

    await waitFor(() => expect(onLoadTree).not.toHaveBeenCalled());

    fireEvent.click(screen.getByRole("button", { name: "Load tree" }));

    await waitFor(() => expect(onLoadTree).toHaveBeenCalledTimes(1));
  });

  it("collapses and expands the file path bar", () => {
    render(
      <EditorPaneHarness
        filePath="README.md"
        toolbarEnd={<button type="button">Close workspace</button>}
      />
    );

    expect(screen.getByLabelText("File path")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Hide file path bar" }));

    expect(screen.queryByLabelText("File path")).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit Markdown" })).toBeNull();
    expect(screen.getByText("README.md")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Close workspace" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Show file path bar" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Show file path bar" }));

    expect(screen.getByLabelText("File path")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Hide file path bar" })).toBeTruthy();
  });

  it("supports externally placed collapse controls", () => {
    render(
      <EditorPaneHarness
        filePath="README.md"
        headerCollapsed
        headerControlsPlacement="external"
        toolbarEnd={<button type="button">Close workspace</button>}
      />
    );

    expect(screen.queryByLabelText("File path")).toBeNull();
    expect(screen.queryByRole("button", { name: "Show file path bar" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Close workspace" })).toBeNull();
    expect(screen.getByTestId("mock-workspace-file-editor")).toBeTruthy();
  });

  it("applies custom editor content spacing", () => {
    render(<EditorPaneHarness filePath="README.md" editorClassName="md:mx-6" />);

    expect(screen.getByTestId("mock-workspace-file-editor").className).toContain("md:mx-6");
  });


  it("switches markdown view modes from the toolbar", async () => {
    render(<EditorPaneHarness filePath="README.md" />);

    expect(screen.getByRole("button", { name: "Edit Markdown" }).getAttribute("aria-pressed"))
      .toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Preview Markdown" }));
    expect(screen.getByRole("button", { name: "Preview Markdown" }).getAttribute("aria-pressed"))
      .toBe("true");
    expect((await screen.findByTestId("mock-workspace-file-editor")).textContent).toBe("preview");

    fireEvent.click(screen.getByRole("button", { name: "Split Markdown view" }));
    expect(screen.getByRole("button", { name: "Split Markdown view" }).getAttribute("aria-pressed"))
      .toBe("true");
    expect(screen.getByTestId("mock-workspace-file-editor").textContent).toBe("split");
  });

  it("hides markdown controls for non-markdown files", () => {
    render(<EditorPaneHarness filePath="src/main.go" />);

    expect(screen.queryByRole("button", { name: "Edit Markdown" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Preview Markdown" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Split Markdown view" })).toBeNull();
  });

  it("selects PDF files without reading them as text", async () => {
    const onReadFile = vi.fn(async () => "should not load");

    render(<PdfLoadHarness onReadFile={onReadFile} />);

    fireEvent.click(screen.getByRole("button", { name: "Open PDF" }));

    await waitFor(() => expect(screen.getByTestId("loaded-path").textContent).toBe("docs/manual.pdf"));
    expect(onReadFile).not.toHaveBeenCalled();
    expect(screen.getByTestId("loaded-body").textContent).toBe("");
  });

  it("downloads files from the toolbar and disables saving PDFs", () => {
    const downloadFile = vi.fn(async () => undefined);

    render(
      <WorkspaceFileEditorPane
        controller={controllerFixture({
          filePath: "docs/manual.pdf",
          downloadFile,
        })}
        theme="dark"
        contentAriaLabel="File content"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Download file" }));

    expect(downloadFile).toHaveBeenCalledTimes(1);
    expect((screen.getByRole("button", { name: "Save file" }) as HTMLButtonElement).disabled)
      .toBe(true);
  });

  it("renders project changes in the file tree pane", async () => {
    const loadGitDiff = vi.fn(async () => undefined);
    const setGitTarget = vi.fn();
    const setGitCompare = vi.fn();

    render(
      <WorkspaceFileTreePaneHarness
        controller={controllerFixture({
          gitEnabled: true,
          workspacePaneView: "changes",
          gitScope: "branch",
          gitTarget: "origin/main",
          gitCompare: "feature",
          gitStatus: {
            available: true,
            scope: "branch",
            branch: "feature",
            target: "origin/main",
            compare: "feature",
            targets: [
              { name: "origin/main", default: true },
              { name: "release" },
              { name: "feature" },
            ],
            changes: [
              { path: "src/main.go", status: "modified", unstaged: true },
            ],
          },
          loadGitDiff,
          setGitTarget,
          setGitCompare,
        })}
      />
    );

    expect(screen.getByRole("button", { name: "Changes" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.change(screen.getByRole("combobox", { name: "Base branch" }), {
      target: { value: "release" },
    });
    expect(setGitTarget).toHaveBeenCalledWith("release");
    fireEvent.change(screen.getByRole("combobox", { name: "Compare branch" }), {
      target: { value: "origin/main" },
    });
    expect(setGitCompare).toHaveBeenCalledWith("origin/main");

    fireEvent.click(screen.getByRole("button", { name: /src\/main.go/i }));

    expect(loadGitDiff).toHaveBeenCalledWith("src/main.go");
  });

  it("renders commit history and commit changed files in the changes pane", () => {
    const selectGitCommit = vi.fn(async () => undefined);
    const setGitHistoryMode = vi.fn();
    const setGitHistoryQuery = vi.fn();
    const loadGitDiff = vi.fn(async () => undefined);
    const commitSHA = "abc1234567890abcdef";

    render(
      <WorkspaceFileTreePaneHarness
        controller={controllerFixture({
          gitEnabled: true,
          workspacePaneView: "changes",
          gitScope: "commit",
          gitHistoryMode: "repository",
          gitSelectedCommit: commitSHA,
          gitHistory: {
            available: true,
            branch: "feature",
            mode: "repository",
            limit: 50,
            offset: 0,
            has_more: false,
            commits: [
              {
                sha: commitSHA,
                short_sha: "abc1234",
                subject: "add history",
                author_name: "Test User",
                author_email: "test@example.com",
                authored_at: "2026-05-23T10:00:00Z",
              },
            ],
          },
          gitStatus: {
            available: true,
            scope: "commit",
            branch: "feature",
            commit: commitSHA,
            base: "def4567890123abcdef",
            changes: [
              { path: "src/history.ts", status: "modified" },
            ],
          },
          selectGitCommit,
          setGitHistoryMode,
          setGitHistoryQuery,
          loadGitDiff,
        })}
      />
    );

    expect(screen.getByRole("button", { name: "Changes" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "Repository" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Current file" }));
    expect(setGitHistoryMode).toHaveBeenCalledWith("file");
    fireEvent.change(screen.getByRole("textbox", { name: "Search commits" }), {
      target: { value: "abc1234" },
    });
    expect(setGitHistoryQuery).toHaveBeenCalledWith("abc1234");

    fireEvent.click(screen.getByText("add history").closest("button")!);
    expect(selectGitCommit).toHaveBeenCalledWith(expect.objectContaining({ sha: commitSHA }));

    fireEvent.click(screen.getByText("src/history.ts").closest("button")!);
    expect(loadGitDiff).toHaveBeenCalledWith("src/history.ts");
  });

  it("does not show stale commit history after the search query changes", async () => {
    const pending: Array<(value: WorkspaceGitHistory) => void> = [];
    const onLoadGitHistory: NonNullable<Parameters<typeof useWorkspaceFileBrowser>[0]["onLoadGitHistory"]> = vi.fn(
      () => new Promise<WorkspaceGitHistory>((resolve) => pending.push(resolve))
    );

    render(<HistoryRaceHarness onLoadGitHistory={onLoadGitHistory} />);

    fireEvent.click(screen.getByRole("button", { name: "Load history" }));
    await waitFor(() => expect(onLoadGitHistory).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: "Search webbbb" }));
    expect(screen.getByTestId("history-query").textContent).toBe("webbbb");

    await act(async () => {
      pending[0]?.({
        available: true,
        branch: "feature",
        mode: "repository",
        limit: 50,
        offset: 0,
        has_more: false,
        commits: [
          {
            sha: "abc1234567890abcdef",
            short_sha: "abc1234",
            subject: "old visible commit",
            author_name: "Test User",
            author_email: "test@example.com",
            authored_at: "2026-05-23T10:00:00Z",
          },
        ],
      });
    });

    expect(screen.queryByText("old visible commit")).toBeNull();
    expect(screen.getByTestId("history-count").textContent).toBe("none");
  });

  it("runs workspace search from the project files pane", async () => {
    const onSearchWorkspace = vi.fn(async () => ({
      query: "needle",
      mode: "content" as const,
      engine: "fallback" as const,
      truncated: false,
      results: [
        {
          path: "src/main.go",
          name: "main.go",
          line_number: 2,
          column: 12,
          preview: "println(\"needle\")",
        },
      ],
    }));

    render(<SearchHarness onSearchWorkspace={onSearchWorkspace} />);

    fireEvent.click(screen.getByRole("button", { name: "Search file content" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Search project files" }), {
      target: { value: "needle" },
    });

    await waitFor(() => expect(onSearchWorkspace).toHaveBeenCalledWith("w1", {
      q: "needle",
      mode: "content",
      case_sensitive: false,
      regex: false,
      whole_word: false,
      limit: 200,
    }));
    expect(await screen.findByRole("button", { name: "src/main.go line 2" })).toBeTruthy();
  });

  it("debounces workspace search while typing", async () => {
    vi.useFakeTimers();
    const onSearchWorkspace = vi.fn(async () => ({
      query: "needle",
      mode: "files" as const,
      engine: "fallback" as const,
      truncated: false,
      results: [],
    }));

    render(<SearchHarness onSearchWorkspace={onSearchWorkspace} />);

    const input = screen.getByRole("textbox", { name: "Search project files" });
    fireEvent.change(input, { target: { value: "n" } });
    await act(async () => {
      vi.advanceTimersByTime(200);
    });
    fireEvent.change(input, { target: { value: "ne" } });
    await act(async () => {
      vi.advanceTimersByTime(300);
    });
    expect(onSearchWorkspace).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(100);
    });
    expect(onSearchWorkspace).toHaveBeenCalledTimes(1);
    expect(onSearchWorkspace).toHaveBeenCalledWith("w1", expect.objectContaining({ q: "ne" }));
  });

  it("handles null search results defensively", async () => {
    const onSearchWorkspace = vi.fn(async () => ({
      query: "missing",
      mode: "files" as const,
      engine: "fallback" as const,
      truncated: false,
      results: null,
    } as any));

    render(<SearchHarness onSearchWorkspace={onSearchWorkspace} />);

    fireEvent.change(screen.getByRole("textbox", { name: "Search project files" }), {
      target: { value: "missing" },
    });

    expect(await screen.findByText("0 results")).toBeTruthy();
    expect(screen.getByText("No results.")).toBeTruthy();
  });

  it("opens search results at the returned content position", () => {
    const loadFile = vi.fn(async () => undefined);
    render(
      <WorkspaceFileTreePaneHarness
        controller={controllerFixture({
          canSearchWorkspace: true,
          searchQuery: "needle",
          searchMode: "content",
          searchResults: [
            {
              path: "src/main.go",
              name: "main.go",
              line_number: 2,
              column: 12,
              preview: "println(\"needle\")",
            },
          ],
          loadFile,
        })}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "src/main.go line 2" }));

    expect(loadFile).toHaveBeenCalledWith("src/main.go", {
      preview: true,
      position: { lineNumber: 2, column: 12 },
    });
  });
});

function WorkspaceFileBrowserHookHarness({
  autoLoadTree,
  onLoadTree,
}: {
  autoLoadTree: boolean;
  onLoadTree: () => Promise<any>;
}) {
  const controller = useWorkspaceFileBrowser({
    workspaceID: "w1",
    workspacePath: "/workspace/AgentX",
    autoLoadTree,
    onLoadTree,
    onReadFile: async () => "",
    onWriteFile: async () => undefined,
    onDeleteFile: async () => undefined,
  });
  return (
    <button type="button" onClick={() => void controller.loadTree()}>
      Load tree
    </button>
  );
}

function EditorPaneHarness({
  filePath,
  toolbarEnd,
  headerCollapsed,
  headerControlsPlacement,
  editorClassName,
}: {
  filePath: string;
  toolbarEnd?: ReactNode;
  headerCollapsed?: boolean;
  headerControlsPlacement?: "pane" | "external";
  editorClassName?: string;
}) {
  const [mode, setMode] = useState<WorkspaceFileViewMode>("edit");
  const [collapsed, setCollapsed] = useState(headerCollapsed ?? false);
  return (
    <WorkspaceFileEditorPane
      controller={controllerFixture({
        filePath,
        fileViewMode: mode,
        setFileViewMode: setMode,
      })}
      theme="dark"
      contentAriaLabel="File content"
      toolbarEnd={toolbarEnd}
      editorClassName={editorClassName}
      headerCollapsed={headerCollapsed === undefined ? undefined : collapsed}
      onHeaderCollapsedChange={headerCollapsed === undefined ? undefined : setCollapsed}
      headerControlsPlacement={headerControlsPlacement}
    />
  );
}

function workspaceTreeFixture() {
  return {
    name: "",
    path: "",
    type: "directory" as const,
    children: [],
    children_loaded: true,
  };
}

function PdfLoadHarness({
  onReadFile,
}: {
  onReadFile: (workspaceID: string, path: string) => Promise<string>;
}) {
  const controller = useWorkspaceFileBrowser({
    workspaceID: "w1",
    workspacePath: "/workspace/AgentX",
    autoLoadTree: false,
    onLoadTree: async () => workspaceTreeFixture(),
    onReadFile,
    onFetchFileBlob: async () => new Blob(["%PDF-1.7"]),
    onWriteFile: async () => undefined,
    onDeleteFile: async () => undefined,
  });
  return (
    <div>
      <button type="button" onClick={() => void controller.loadFile("docs/manual.pdf")}>
        Open PDF
      </button>
      <div data-testid="loaded-path">{controller.filePath}</div>
      <div data-testid="loaded-body">{controller.fileBody}</div>
    </div>
  );
}

function HistoryRaceHarness({
  onLoadGitHistory,
}: {
  onLoadGitHistory: NonNullable<Parameters<typeof useWorkspaceFileBrowser>[0]["onLoadGitHistory"]>;
}) {
  const controller = useWorkspaceFileBrowser({
    workspaceID: "w1",
    workspacePath: "/workspace/AgentX",
    autoLoadTree: false,
    onLoadTree: async () => workspaceTreeFixture(),
    onReadFile: async () => "",
    onWriteFile: async () => undefined,
    onDeleteFile: async () => undefined,
    onLoadGitStatus: async () => ({
      available: true,
      scope: "commit",
      changes: [],
    }),
    onLoadGitHistory,
    onLoadGitDiff: async () => ({
      scope: "commit",
      path: "README.md",
      status: "modified",
      original: "",
      modified: "",
    }),
  });
  return (
    <div>
      <button type="button" onClick={() => void controller.loadGitHistory()}>
        Load history
      </button>
      <button type="button" onClick={() => controller.setGitHistoryQuery("webbbb")}>
        Search webbbb
      </button>
      <div data-testid="history-query">{controller.gitHistoryQuery}</div>
      <div data-testid="history-count">
        {controller.gitHistory ? String(controller.gitHistory.commits.length) : "none"}
      </div>
      {controller.gitHistory?.commits.map((commit) => (
        <div key={commit.sha}>{commit.subject}</div>
      ))}
    </div>
  );
}

function SearchHarness({
  onSearchWorkspace,
}: {
  onSearchWorkspace: NonNullable<Parameters<typeof useWorkspaceFileBrowser>[0]["onSearchWorkspace"]>;
}) {
  const controller = useWorkspaceFileBrowser({
    workspaceID: "w1",
    workspacePath: "/workspace/AgentX",
    autoLoadTree: false,
    onLoadTree: async () => workspaceTreeFixture(),
    onSearchWorkspace,
    onReadFile: async () => "",
    onWriteFile: async () => undefined,
    onDeleteFile: async () => undefined,
  });
  return <WorkspaceFileTreePaneHarness controller={controller} />;
}

function WorkspaceFileTreePaneHarness({
  controller,
}: {
  controller: WorkspaceFileBrowserController;
}) {
  return (
    <WorkspaceFileTreePane
      controller={controller}
      title="Project files"
      ariaLabel="Project files"
    />
  );
}

function controllerFixture(
  overrides: Partial<WorkspaceFileBrowserController> = {}
): WorkspaceFileBrowserController {
  const filePath = overrides.filePath ?? "README.md";
  return {
    workspaceID: "w1",
    workspacePath: "/workspace/AgentX",
    filePath,
    fileBody: "",
    tree: undefined,
    workspaceTreeResetKey: 0,
    workspaceTreeLoading: false,
    workspaceTreeError: null,
    directoryLoadingPaths: new Set(),
    directoryLoadErrors: {},
    searchQuery: "",
    searchMode: "files",
    searchCaseSensitive: false,
    searchRegex: false,
    searchWholeWord: false,
    searchLoading: false,
    searchError: null,
    searchResults: [],
    searchTruncated: false,
    searchEngine: undefined,
    fileLoading: false,
    fileLoadError: null,
    fileSaving: false,
    fileDownloading: false,
    fileDeleting: false,
    entryActionPending: false,
    workspaceStatus: null,
    workspacePaneView: "files",
    gitEnabled: false,
    gitScope: "working_tree",
    gitTarget: "",
    gitCompare: "",
    gitHistoryMode: "repository",
    gitHistoryQuery: "",
    gitHistory: undefined,
    gitHistoryLoading: false,
    gitHistoryError: null,
    gitSelectedCommit: "",
    gitStatus: undefined,
    gitStatusLoading: false,
    gitStatusError: null,
    gitDiff: undefined,
    gitDiffLoading: false,
    gitDiffError: null,
    gitSelectedPath: "",
    fileOpenPosition: undefined,
    fileOpenRequestID: 0,
    fileViewMode: "edit",
    canUseWorkspace: true,
    canFetchFileBlob: true,
    canSearchWorkspace: false,
    tabs: [],
    activeTabId: null,
    activeTabEditorViewState: null,
    activeTabMarkdownPreviewScrollTop: 0,
    switchTab: vi.fn(),
    closeTab: vi.fn(),
    closeOtherTabs: vi.fn(),
    closeAllTabs: vi.fn(),
    pinTab: vi.fn(),
    reorderTabs: vi.fn(),
    setActiveTabEditorViewState: vi.fn(),
    saveTabEditorViewState: vi.fn(),
    saveTabMarkdownPreviewScrollTop: vi.fn(),
    setFilePath: vi.fn(),
    setFileBody: vi.fn(),
    setSearchQuery: vi.fn(),
    setSearchMode: vi.fn(),
    setSearchCaseSensitive: vi.fn(),
    setSearchRegex: vi.fn(),
    setSearchWholeWord: vi.fn(),
    setFileViewMode: vi.fn(),
    setWorkspacePaneView: vi.fn(),
    setGitScope: vi.fn(),
    setGitTarget: vi.fn(),
    setGitCompare: vi.fn(),
    setGitHistoryMode: vi.fn(),
    setGitHistoryQuery: vi.fn(),
    loadTree: vi.fn(async () => undefined),
    loadDirectory: vi.fn(async () => undefined),
    loadSearch: vi.fn(async () => undefined),
    clearSearch: vi.fn(),
    loadFile: vi.fn(async () => undefined),
    loadGitStatus: vi.fn(async () => undefined),
    loadGitHistory: vi.fn(async () => undefined),
    selectGitCommit: vi.fn(async () => undefined),
    loadGitDiff: vi.fn(async () => undefined),
    saveFile: vi.fn(async () => undefined),
    fetchFileBlob: vi.fn(async () => new Blob(["file"])),
    downloadFile: vi.fn(async () => undefined),
    deleteFile: vi.fn(async () => undefined),
    createEntry: vi.fn(async () => null),
    renameEntry: vi.fn(async () => null),
    deleteEntry: vi.fn(async () => undefined),
    moveEntry: vi.fn(async () => null),
    ...overrides,
    trimmedPath: (overrides.trimmedPath ?? filePath).trim(),
  };
}
