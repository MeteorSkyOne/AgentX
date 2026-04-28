import { describe, expect, it } from "vitest";
import {
  parseWorkspacePathTarget,
  splitWorkspacePathTargets,
  type WorkspacePathTarget,
} from "./workspacePaths";

const workspacePath = "/home/meteorsky/code/AgentX";

describe("parseWorkspacePathTarget", () => {
  it("parses relative workspace paths", () => {
    expect(parseWorkspacePathTarget("web/src/App.tsx", workspacePath)).toEqual({
      path: "web/src/App.tsx",
      label: "web/src/App.tsx",
    });
    expect(parseWorkspacePathTarget("./internal/app/app.go", workspacePath)).toEqual({
      path: "internal/app/app.go",
      label: "./internal/app/app.go",
    });
  });

  it("parses root filenames", () => {
    expect(parseWorkspacePathTarget("AGENTS.md", workspacePath)).toEqual({
      path: "AGENTS.md",
      label: "AGENTS.md",
    });
    expect(parseWorkspacePathTarget("package.json", workspacePath)).toEqual({
      path: "package.json",
      label: "package.json",
    });
  });

  it("converts workspace absolute paths to relative targets", () => {
    expect(
      parseWorkspacePathTarget(
        "/home/meteorsky/code/AgentX/web/src/App.tsx:12:3,",
        workspacePath
      )
    ).toEqual({
      path: "web/src/App.tsx",
      lineNumber: 12,
      column: 3,
      label: "/home/meteorsky/code/AgentX/web/src/App.tsx:12:3",
    });
  });

  it("uses column one when a line is present without a column", () => {
    expect(parseWorkspacePathTarget("web/src/App.tsx:12", workspacePath)).toEqual({
      path: "web/src/App.tsx",
      lineNumber: 12,
      column: 1,
      label: "web/src/App.tsx:12",
    });
  });

  it("strips common trailing punctuation", () => {
    expect(parseWorkspacePathTarget("web/src/App.tsx:12).", workspacePath)).toEqual({
      path: "web/src/App.tsx",
      lineNumber: 12,
      column: 1,
      label: "web/src/App.tsx:12",
    });
  });

  it("rejects external and unsupported references", () => {
    expect(parseWorkspacePathTarget("https://example.com/web/src/App.tsx", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("mailto:dev@example.com", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("tel:+15551234567", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("web/src/App.tsx?raw", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("web/src/App.tsx#L10", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("web/**/*.tsx", workspacePath)).toBeNull();
  });

  it("rejects package-like or version-like strings", () => {
    expect(parseWorkspacePathTarget("react-dom/client", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("node_modules/lodash", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("lodash/fp", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("v1.2.3", workspacePath)).toBeNull();
  });

  it("rejects paths that escape or sit outside the workspace", () => {
    expect(parseWorkspacePathTarget("../secrets.txt", workspacePath)).toBeNull();
    expect(parseWorkspacePathTarget("web/../secrets.txt", workspacePath)).toBeNull();
    expect(
      parseWorkspacePathTarget("/home/meteorsky/code/Other/web/src/App.tsx", workspacePath)
    ).toBeNull();
  });

  it("requires a workspace root", () => {
    expect(parseWorkspacePathTarget("web/src/App.tsx", undefined)).toBeNull();
  });
});

describe("splitWorkspacePathTargets", () => {
  it("returns plain text for empty or whitespace input", () => {
    expect(splitWorkspacePathTargets("", workspacePath)).toEqual([""]);
    expect(splitWorkspacePathTargets("  ", workspacePath)).toEqual(["  "]);
  });

  it("parses path-only input", () => {
    expect(splitWorkspacePathTargets("web/src/App.tsx", workspacePath)).toEqual([
      {
        path: "web/src/App.tsx",
        label: "web/src/App.tsx",
      } satisfies WorkspacePathTarget,
    ]);
  });

  it("keeps surrounding punctuation as text", () => {
    expect(splitWorkspacePathTargets("Open web/src/App.tsx:4.", workspacePath)).toEqual([
      "Open ",
      {
        path: "web/src/App.tsx",
        lineNumber: 4,
        column: 1,
        label: "web/src/App.tsx:4",
      } satisfies WorkspacePathTarget,
      ".",
    ]);
  });

  it("parses multiple and adjacent paths", () => {
    expect(
      splitWorkspacePathTargets("See web/src/A.tsx and internal/app/app.go", workspacePath)
    ).toEqual([
      "See ",
      {
        path: "web/src/A.tsx",
        label: "web/src/A.tsx",
      } satisfies WorkspacePathTarget,
      " and ",
      {
        path: "internal/app/app.go",
        label: "internal/app/app.go",
      } satisfies WorkspacePathTarget,
    ]);

    expect(splitWorkspacePathTargets("web/a.ts web/b.ts", workspacePath)).toEqual([
      {
        path: "web/a.ts",
        label: "web/a.ts",
      } satisfies WorkspacePathTarget,
      " ",
      {
        path: "web/b.ts",
        label: "web/b.ts",
      } satisfies WorkspacePathTarget,
    ]);
  });

  it("parses paths at the start and end of text", () => {
    expect(splitWorkspacePathTargets("web/src/App.tsx is the file", workspacePath)).toEqual([
      {
        path: "web/src/App.tsx",
        label: "web/src/App.tsx",
      } satisfies WorkspacePathTarget,
      " is the file",
    ]);
    expect(splitWorkspacePathTargets("the file is web/src/App.tsx", workspacePath)).toEqual([
      "the file is ",
      {
        path: "web/src/App.tsx",
        label: "web/src/App.tsx",
      } satisfies WorkspacePathTarget,
    ]);
  });

  it("parses embedded workspace absolute paths", () => {
    expect(
      splitWorkspacePathTargets(
        "Editing /home/meteorsky/code/AgentX/web/src/App.tsx now",
        workspacePath
      )
    ).toEqual([
      "Editing ",
      {
        path: "web/src/App.tsx",
        label: "/home/meteorsky/code/AgentX/web/src/App.tsx",
      } satisfies WorkspacePathTarget,
      " now",
    ]);
  });

  it("does not partially link query or hash references", () => {
    expect(splitWorkspacePathTargets("Open web/src/App.tsx?raw", workspacePath)).toEqual([
      "Open web/src/App.tsx?raw",
    ]);
    expect(splitWorkspacePathTargets("Open web/src/App.tsx#L1", workspacePath)).toEqual([
      "Open web/src/App.tsx#L1",
    ]);
    expect(splitWorkspacePathTargets("Open web/src/App.tsx:0", workspacePath)).toEqual([
      "Open web/src/App.tsx:0",
    ]);
    expect(splitWorkspacePathTargets("Open web/src/App.tsx=1", workspacePath)).toEqual([
      "Open web/src/App.tsx=1",
    ]);
  });

  it("does not link package names or assignment values", () => {
    expect(splitWorkspacePathTargets("Import react-dom/client", workspacePath)).toEqual([
      "Import react-dom/client",
    ]);
    expect(splitWorkspacePathTargets("Install @types/react", workspacePath)).toEqual([
      "Install @types/react",
    ]);
    expect(splitWorkspacePathTargets("key=web/src/foo.ts", workspacePath)).toEqual([
      "key=web/src/foo.ts",
    ]);
  });
});
