// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceFileEditor } from "./WorkspaceFileEditor";
import type { WorkspaceFileBrowserController } from "./WorkspaceFileBrowser";

vi.mock("@/lib/monaco", () => ({}));

vi.mock("@monaco-editor/react", () => ({
  default: ({ value }: { value: string }) => (
    <div data-testid="mock-monaco-editor">{value}</div>
  ),
}));

afterEach(() => {
  cleanup();
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
    fileOpenPosition: undefined,
    fileOpenRequestID: 0,
    fileViewMode: "edit",
    canUseWorkspace: true,
    setFilePath: vi.fn(),
    setFileBody: vi.fn(),
    setFileViewMode: vi.fn(),
    loadTree: vi.fn(async () => undefined),
    loadDirectory: vi.fn(async () => undefined),
    loadFile: vi.fn(async () => undefined),
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
