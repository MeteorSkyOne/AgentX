import type { Selection, editor } from "monaco-editor";
import { describe, expect, it } from "vitest";
import { workspaceEditorCopyText } from "./workspaceEditorCopy";

function selection(startLineNumber: number, startColumn: number, endLineNumber = startLineNumber, endColumn = startColumn) {
  return {
    startLineNumber, startColumn, endLineNumber, endColumn,
    isEmpty: () => startLineNumber === endLineNumber && startColumn === endColumn,
  } as Selection;
}

function editorFixture(content: string, selections: Selection[]) {
  const lines = content.split(/\r\n|\n/);
  return {
    getSelections: () => selections,
    getModel: () => ({
      getLineContent: (line: number) => lines[line - 1],
      getValueInRange: (range: Selection) => lines
        .slice(range.startLineNumber - 1, range.endLineNumber)
        .map((line, index) => {
          const start = index === 0 ? range.startColumn - 1 : 0;
          const end = index === range.endLineNumber - range.startLineNumber
            ? range.endColumn - 1 : line.length;
          return line.slice(start, end);
        }).join(content.includes("\r\n") ? "\r\n" : "\n"),
    }),
  } as unknown as editor.ICodeEditor;
}

describe("workspaceEditorCopyText", () => {
  it("copies the current line when there is no selection", () => {
    const instance = editorFixture("first\n  second\nthird", [selection(2, 4)]);
    expect(workspaceEditorCopyText(instance, false, "src/example.ts")).toBe("src/example.ts:2");
    expect(workspaceEditorCopyText(instance, true, "src/example.ts")).toBe("src/example.ts:2\n2:   second");
  });

  it("preserves partial selections and original line numbers", () => {
    const instance = editorFixture("first\n  second\nthird", [selection(2, 3, 3, 4)]);
    expect(workspaceEditorCopyText(instance, false, "src/example.ts")).toBe("src/example.ts:2-3");
    expect(workspaceEditorCopyText(instance, true, "src/example.ts")).toBe("src/example.ts:2-3\n2: second\n3: thi");
  });

  it("excludes the next line when the selection ends at column one", () => {
    const instance = editorFixture("first\nsecond\nthird", [selection(1, 1, 3, 1)]);
    expect(workspaceEditorCopyText(instance, false, "src/example.ts")).toBe("src/example.ts:1-2");
    expect(workspaceEditorCopyText(instance, true, "src/example.ts")).toBe("src/example.ts:1-2\n1: first\n2: second");
  });

  it("handles CRLF content and empty lines", () => {
    const instance = editorFixture("first\r\n\r\nthird", [selection(1, 1, 3, 6)]);
    expect(workspaceEditorCopyText(instance, true, "src/example.ts")).toBe("src/example.ts:1-3\n1: first\n2: \n3: third");
  });

  it("preserves separate selections and cursor positions", () => {
    const instance = editorFixture("first\nsecond\nthird", [selection(1, 2, 1, 4), selection(3, 1)]);
    expect(workspaceEditorCopyText(instance, false, "src/example.ts")).toBe("src/example.ts:1\nsrc/example.ts:3");
    expect(workspaceEditorCopyText(instance, true, "src/example.ts")).toBe("src/example.ts:1\n1: ir\nsrc/example.ts:3\n3: third");
  });

  it("does not copy when there is no active model or selection", () => {
    const instance = editorFixture("", []);
    expect(workspaceEditorCopyText(instance, true, "src/example.ts")).toBeNull();
    instance.getModel = () => null;
    expect(workspaceEditorCopyText(instance, false, "src/example.ts")).toBeNull();
  });
});
