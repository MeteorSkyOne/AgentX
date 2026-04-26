import { describe, expect, it } from "vitest";
import type { FileTreeEntry } from "./FileTree";
import { defaultOpenDirectoryPaths, fileTreeHasRows, visibleFileTreeRows } from "./FileTree";

describe("FileTree helpers", () => {
  it("does not expose the empty workspace root as a visible row", () => {
    const rows = visibleFileTreeRows(workspaceTree(), new Set(defaultOpenDirectoryPaths(workspaceTree())));

    expect(rows.map((row) => row.path)).toEqual([
      "src",
      "src/components",
      "src/components/FileTree.tsx",
      "README.md"
    ]);
  });

  it("defaults nested directories to open", () => {
    expect(defaultOpenDirectoryPaths(workspaceTree())).toEqual(["src", "src/components"]);
  });

  it("hides children of closed directories", () => {
    const rows = visibleFileTreeRows(workspaceTree(), new Set());

    expect(rows.map((row) => row.path)).toEqual(["src", "README.md"]);
  });

  it("treats an empty workspace root as empty", () => {
    expect(fileTreeHasRows({ name: "", path: "", type: "directory" })).toBe(false);
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
        children: [
          {
            name: "components",
            path: "src/components",
            type: "directory",
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
