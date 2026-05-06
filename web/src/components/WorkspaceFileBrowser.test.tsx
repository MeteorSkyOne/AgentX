// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
    setFilePath: vi.fn(),
    setFileBody: vi.fn(),
    setFileViewMode: vi.fn(),
    setWorkspacePaneView: vi.fn(),
    setGitScope: vi.fn(),
    setGitTarget: vi.fn(),
    setGitCompare: vi.fn(),
    loadTree: vi.fn(async () => undefined),
    loadDirectory: vi.fn(async () => undefined),
    loadFile: vi.fn(async () => undefined),
    loadGitStatus: vi.fn(async () => undefined),
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
