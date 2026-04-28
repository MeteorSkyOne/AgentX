// @vitest-environment jsdom

import { createElement } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeEntry } from "./FileTree";
import { FileTree, defaultOpenDirectoryPaths, fileTreeHasRows, visibleFileTreeRows } from "./FileTree";

afterEach(() => {
  cleanup();
});

describe("FileTree helpers", () => {
  it("does not expose the empty workspace root as a visible row", () => {
    const rows = visibleFileTreeRows(workspaceTree(), new Set(defaultOpenDirectoryPaths(workspaceTree())));

    expect(rows.map((row) => row.path)).toEqual(["src", "README.md"]);
  });

  it("defaults directories to closed", () => {
    expect(defaultOpenDirectoryPaths(workspaceTree())).toEqual([]);
  });

  it("hides children of closed directories", () => {
    const rows = visibleFileTreeRows(workspaceTree(), new Set());

    expect(rows.map((row) => row.path)).toEqual(["src", "README.md"]);
  });

  it("treats an empty workspace root as empty", () => {
    expect(fileTreeHasRows({ name: "", path: "", type: "directory" })).toBe(false);
  });

  it("requests unloaded directories when they are opened", () => {
    const onLoadDirectory = vi.fn();

    render(
      createElement(FileTree, {
        tree: lazyWorkspaceTree(),
        onSelectFile: vi.fn(),
        onLoadDirectory,
      })
    );

    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));

    expect(onLoadDirectory).toHaveBeenCalledWith(
      "src",
      expect.objectContaining({ path: "src" })
    );
  });

  it("does not request already loaded directories when they are opened", () => {
    const onLoadDirectory = vi.fn();

    render(
      createElement(FileTree, {
        tree: workspaceTree(),
        onSelectFile: vi.fn(),
        onLoadDirectory,
      })
    );

    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));

    expect(onLoadDirectory).not.toHaveBeenCalled();
    expect(screen.getByRole("treeitem", { name: "components" })).toBeTruthy();
  });

  it("moves a dragged entry onto a directory", () => {
    const onMoveEntry = vi.fn();
    const transfer = dragDataTransfer();

    render(
      createElement(FileTree, {
        tree: workspaceTree(),
        onSelectFile: vi.fn(),
        onMoveEntry,
      })
    );

    fireEvent.dragStart(screen.getByRole("treeitem", { name: "README.md" }), {
      dataTransfer: transfer,
    });
    fireEvent.dragOver(screen.getByRole("treeitem", { name: "src" }), {
      dataTransfer: transfer,
    });
    fireEvent.drop(screen.getByRole("treeitem", { name: "src" }), {
      dataTransfer: transfer,
    });

    expect(onMoveEntry).toHaveBeenCalledWith(
      expect.objectContaining({ path: "README.md" }),
      "src"
    );
  });

  it("opens the root context menu from blank tree space", async () => {
    const onCreateEntry = vi.fn();

    render(
      createElement(FileTree, {
        tree: workspaceTree(),
        onSelectFile: vi.fn(),
        onCreateEntry,
      })
    );

    const treeTrigger = screen.getByRole("tree").closest("[data-slot='context-menu-trigger']");
    expect(treeTrigger).toBeTruthy();

    fireEvent.contextMenu(treeTrigger as HTMLElement);
    fireEvent.click(await screen.findByRole("menuitem", { name: "New file" }));

    expect(onCreateEntry).toHaveBeenCalledWith("", "file");
  });
});

function workspaceTree(): FileTreeEntry {
  return {
    name: "",
    path: "",
    type: "directory",
    children: [
      {
        name: "src",
        path: "src",
        type: "directory",
        children_loaded: true,
        children: [
          {
            name: "components",
            path: "src/components",
            type: "directory",
            children_loaded: true,
            children: [
              {
                name: "FileTree.tsx",
                path: "src/components/FileTree.tsx",
                type: "file"
              }
            ]
          }
        ]
      },
      {
        name: "README.md",
        path: "README.md",
        type: "file"
      }
    ]
  };
}

function lazyWorkspaceTree(): FileTreeEntry {
  return {
    name: "",
    path: "",
    type: "directory",
    children_loaded: true,
    children: [
      {
        name: "src",
        path: "src",
        type: "directory",
        has_children: true
      },
      {
        name: "README.md",
        path: "README.md",
        type: "file"
      }
    ]
  };
}

function dragDataTransfer(): DataTransfer {
  const values = new Map<string, string>();
  return {
    dropEffect: "none",
    effectAllowed: "all",
    files: [] as unknown as FileList,
    items: [] as unknown as DataTransferItemList,
    types: [],
    clearData: vi.fn((type?: string) => {
      if (type) {
        values.delete(type);
        return;
      }
      values.clear();
    }),
    getData: vi.fn((type: string) => values.get(type) ?? ""),
    setData: vi.fn((type: string, value: string) => {
      values.set(type, value);
    }),
    setDragImage: vi.fn(),
  };
}
