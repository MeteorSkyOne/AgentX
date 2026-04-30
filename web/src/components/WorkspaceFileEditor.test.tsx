// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { cloneElement, type ComponentProps, type ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceFileEditor, WorkspaceGitDiffViewer } from "./WorkspaceFileEditor";
import type { WorkspaceFileBrowserController } from "./WorkspaceFileBrowser";

vi.mock("@/lib/monaco", () => ({}));

const diffEditorState = vi.hoisted(() => ({
  mounts: 0,
  navigationCalls: [] as string[],
}));

vi.mock("@monaco-editor/react", async () => {
  const React = await import("react");

  return {
    default: ({ value }: { value: string }) => (
      <div data-testid="mock-monaco-editor">{value}</div>
    ),
    DiffEditor: ({
      original,
      modified,
      options,
      onMount,
    }: {
      original: string;
      modified: string;
      options?: Record<string, unknown>;
      onMount?: (editor: {
        getLineChanges: () => unknown[];
        goToDiff: (target: "next" | "previous") => void;
        onDidUpdateDiff: (listener: () => void) => { dispose: () => void };
        onDidDispose: (listener: () => void) => { dispose: () => void };
      }, monaco: Record<string, never>) => void;
    }) => {
      React.useEffect(() => {
        diffEditorState.mounts += 1;
        onMount?.({
          getLineChanges: () => [{ modifiedStartLineNumber: 1 }],
          goToDiff: (target) => {
            diffEditorState.navigationCalls.push(target);
          },
          onDidUpdateDiff: (listener) => {
            const timer = window.setTimeout(listener, 0);
            return { dispose: () => window.clearTimeout(timer) };
          },
          onDidDispose: () => ({ dispose: () => undefined }),
        }, {});
      }, [onMount]);

      return (
        <div data-testid="mock-monaco-diff-editor">
          <span>{original}</span>
          <span>{modified}</span>
          <span data-testid="mock-monaco-diff-options">{JSON.stringify(options)}</span>
        </div>
      );
    },
  };
});

afterEach(() => {
  cleanup();
  document.querySelectorAll("[data-testid='workspace-git-diff-navigation-host']").forEach((node) => {
    node.remove();
  });
  diffEditorState.mounts = 0;
  diffEditorState.navigationCalls = [];
});

describe("WorkspaceFileEditor markdown preview", () => {
  it("renders unsaved markdown content in preview mode", () => {
    renderEditor({
      filePath: "docs/readme.md",
      fileBody: "# Draft title\n\nUnsaved body",
      fileViewMode: "preview",
    });

    expect(screen.queryByTestId("mock-monaco-editor")).toBeNull();
    expect(screen.getByTestId("workspace-file-markdown-preview").textContent).toContain(
      "Draft title"
    );
    expect(screen.getByText("Unsaved body")).toBeTruthy();
  });

  it("renders editor and preview together in split mode", () => {
    renderEditor({
      filePath: "docs/readme.md",
      fileBody: "# Split title",
      fileViewMode: "split",
    });

    expect(screen.getByTestId("mock-monaco-editor").textContent).toBe("# Split title");
    expect(screen.getByTestId("workspace-file-markdown-preview").textContent).toContain(
      "Split title"
    );
  });

  it("opens preview links in the same workspace editor", () => {
    const loadFile = vi.fn(async () => undefined);
    renderEditor({
      filePath: "docs/readme.md",
      fileBody: "[Next](next.md:4)",
      fileViewMode: "preview",
      loadFile,
    });

    fireEvent.click(screen.getByRole("button", { name: "Open docs/next.md:4:1" }));

    expect(loadFile).toHaveBeenCalledWith("docs/next.md", {
      position: { lineNumber: 4, column: 1 },
    });
  });

  it("keeps non-markdown files in edit mode", () => {
    renderEditor({
      filePath: "src/main.go",
      fileBody: "package main",
      fileViewMode: "preview",
    });

    expect(screen.getByTestId("mock-monaco-editor")).toBeTruthy();
    expect(screen.queryByTestId("workspace-file-markdown-preview")).toBeNull();
  });
});

describe("WorkspaceGitDiffViewer", () => {
  it("keeps diff lines unwrapped and collapses unchanged regions", () => {
    renderDiffViewer(
      <WorkspaceGitDiffViewer
        diff={{
          scope: "branch",
          branch: "main",
          base: "origin/main",
          target: "origin/main",
          path: "go.mod",
          status: "modified",
          original: "module example\n",
          modified: "module example\n",
        }}
        theme="dark"
        contentAriaLabel="Git diff"
      />
    );

    const options = JSON.parse(screen.getByTestId("mock-monaco-diff-options").textContent ?? "{}");
    expect(options.wordWrap).toBe("off");
    expect(options.diffWordWrap).toBe("off");
    expect(options.hideUnchangedRegions).toEqual({
      enabled: true,
      contextLineCount: 3,
      minimumLineCount: 8,
      revealLineCount: 20,
    });
    const navigationHost = screen.getByTestId("workspace-git-diff-navigation-host");
    expect(navigationHost.className).toContain("shrink-0");
    expect(navigationHost.className).not.toContain("absolute");
  });

  it("remounts the Monaco diff editor when switching changed files", () => {
    const firstDiff = {
      scope: "working_tree" as const,
      branch: "main",
      path: "internal/httpapi/httpapi_test.go",
      status: "modified" as const,
      original: "package httpapi\n",
      modified: "package httpapi\n",
    };
    const { rerender } = renderDiffViewer(
      <WorkspaceGitDiffViewer
        diff={firstDiff}
        theme="dark"
        contentAriaLabel="Git diff"
      />
    );

    expect(diffEditorState.mounts).toBe(1);

    rerender(
      <WorkspaceGitDiffViewer
        diff={{
          ...firstDiff,
          path: "web/src/App.tsx",
          original: "export function App() {}\n",
          modified: "export function App() { return null }\n",
        }}
        theme="dark"
        contentAriaLabel="Git diff"
      />
    );

    expect(diffEditorState.mounts).toBe(2);
  });

  it("navigates to previous and next diff changes", async () => {
    renderDiffViewer(
      <WorkspaceGitDiffViewer
        diff={{
          scope: "working_tree",
          branch: "main",
          path: "web/src/App.tsx",
          status: "modified",
          original: "const before = 1\n",
          modified: "const after = 1\n",
        }}
        theme="dark"
        contentAriaLabel="Git diff"
      />
    );

    const previousButton = screen.getByRole("button", { name: "Previous change" });
    const nextButton = screen.getByRole("button", { name: "Next change" });
    await waitFor(() => {
      expect((previousButton as HTMLButtonElement).disabled).toBe(false);
      expect((nextButton as HTMLButtonElement).disabled).toBe(false);
    });

    fireEvent.click(previousButton);
    fireEvent.click(nextButton);

    expect(diffEditorState.navigationCalls).toEqual(["previous", "next"]);
  });
});

function renderDiffViewer(
  element: ReactElement<ComponentProps<typeof WorkspaceGitDiffViewer>>
) {
  const navigationContainer = document.createElement("div");
  navigationContainer.setAttribute("data-testid", "workspace-git-diff-navigation-host");
  navigationContainer.className = "flex shrink-0 items-center";
  document.body.appendChild(navigationContainer);
  const result = render(cloneElement(element, { navigationContainer }));
  return {
    ...result,
    rerender: (nextElement: ReactElement<ComponentProps<typeof WorkspaceGitDiffViewer>>) => {
      result.rerender(cloneElement(nextElement, { navigationContainer }));
    },
  };
}

function renderEditor(overrides: Partial<WorkspaceFileBrowserController> = {}) {
  const controller = controllerFixture(overrides);
  render(
    <WorkspaceFileEditor
      controller={controller}
      theme="dark"
      contentAriaLabel="File content"
      className="h-96"
    />
  );
}

function controllerFixture(
  overrides: Partial<WorkspaceFileBrowserController> = {}
): WorkspaceFileBrowserController {
  const filePath = overrides.filePath ?? "docs/readme.md";
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
    deleteFile: vi.fn(async () => undefined),
    createEntry: vi.fn(async () => null),
    renameEntry: vi.fn(async () => null),
    deleteEntry: vi.fn(async () => undefined),
    moveEntry: vi.fn(async () => null),
    ...overrides,
    trimmedPath: (overrides.trimmedPath ?? filePath).trim(),
  };
}
