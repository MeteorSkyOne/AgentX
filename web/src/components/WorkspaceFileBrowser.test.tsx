// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  WorkspaceFileEditorPane,
  type WorkspaceFileBrowserController,
  type WorkspaceFileViewMode,
} from "./WorkspaceFileBrowser";

vi.mock("./WorkspaceFileEditor", () => ({
  WorkspaceFileEditor: ({
    controller,
  }: {
    controller: WorkspaceFileBrowserController;
  }) => <div data-testid="mock-workspace-file-editor">{controller.fileViewMode}</div>,
}));

afterEach(() => {
  cleanup();
});

describe("WorkspaceFileEditorPane markdown controls", () => {
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
});

function EditorPaneHarness({
  filePath,
  toolbarEnd,
  headerCollapsed,
  headerControlsPlacement,
}: {
  filePath: string;
  toolbarEnd?: ReactNode;
  headerCollapsed?: boolean;
  headerControlsPlacement?: "pane" | "external";
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
      headerCollapsed={headerCollapsed === undefined ? undefined : collapsed}
      onHeaderCollapsedChange={headerCollapsed === undefined ? undefined : setCollapsed}
      headerControlsPlacement={headerControlsPlacement}
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
