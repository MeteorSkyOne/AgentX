import type { editor } from "monaco-editor";

export function workspaceEditorCopyText(
  editorInstance: editor.ICodeEditor,
  includeContent: boolean,
  filePath: string
): string | null {
  const model = editorInstance.getModel();
  const selections = editorInstance.getSelections();
  if (!model || !selections?.length) return null;

  return selections.map((selection) => {
    const start = selection.startLineNumber;
    // A selection ending at column 1 does not include that line's content.
    const end = selection.endLineNumber > start && selection.endColumn === 1
      ? selection.endLineNumber - 1
      : selection.endLineNumber;
    const location = `${filePath}:${start === end ? start : `${start}-${end}`}`;
    if (!includeContent) return location;

    const content = selection.isEmpty()
      ? model.getLineContent(start)
      : model.getValueInRange(selection);
    const lines = content.split(/\r\n|\r|\n/).slice(0, end - start + 1);
    return `${location}\n${lines.map((line, index) => `${start + index}: ${line}`).join("\n")}`;
  }).join("\n");
}
