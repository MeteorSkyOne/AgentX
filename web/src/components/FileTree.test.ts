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
