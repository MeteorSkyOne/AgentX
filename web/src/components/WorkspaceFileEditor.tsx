import { useCallback, useEffect, useMemo, useRef } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import "@/lib/monaco";
import type { WorkspaceFileEditorProps } from "./WorkspaceFileBrowser";
import { cn } from "@/lib/utils";
import { monacoLanguageForPath } from "./workspaceFileLanguages";

type WorkspaceEditorNode = HTMLDivElement & {
  __agentxSetEditorValue?: (value: string) => void;
  __agentxGetEditorValue?: () => string;
};

export function WorkspaceFileEditor({
  controller,
  theme,
  contentAriaLabel,
  className,
}: WorkspaceFileEditorProps) {
  const saveFileRef = useRef<() => void>(() => undefined);
  const editorContainerRef = useRef<WorkspaceEditorNode | null>(null);
  const editorTheme = theme === "dark" ? "vs-dark" : "light";
  const language = useMemo(() => monacoLanguageForPath(controller.filePath), [controller.filePath]);
  const editorModelPath = useMemo(() => {
    const path = controller.trimmedPath || "untitled";
    const encodedPath = path.split("/").map(encodeURIComponent).join("/");
    return `agentx://workspace/${encodeURIComponent(controller.workspaceID ?? "none")}/${encodedPath}`;
  }, [controller.trimmedPath, controller.workspaceID]);
  const editorOptions = useMemo<editor.IStandaloneEditorConstructionOptions>(
    () => ({
      ariaLabel: contentAriaLabel,
      automaticLayout: true,
      fontSize: 13,
      minimap: { enabled: false },
      readOnly: !controller.canUseWorkspace || controller.fileLoading,
      scrollBeyondLastLine: false,
      wordWrap: "on",
    }),
    [contentAriaLabel, controller.canUseWorkspace, controller.fileLoading]
  );

  useEffect(() => {
    saveFileRef.current = () => {
      void controller.saveFile();
    };
  }, [controller]);

  const handleEditorMount = useCallback<OnMount>(
    (editorInstance, monacoInstance) => {
      if (import.meta.env.DEV) {
        const editorNodes = [
          editorContainerRef.current,
          editorInstance.getDomNode() as WorkspaceEditorNode | null,
        ].filter((node): node is WorkspaceEditorNode => Boolean(node));
        for (const editorNode of editorNodes) {
          editorNode.__agentxSetEditorValue = (value) => {
            editorInstance.setValue(value);
            controller.setFileBody(value);
          };
          editorNode.__agentxGetEditorValue = () => editorInstance.getValue();
        }
        editorInstance.onDidDispose(() => {
          for (const editorNode of editorNodes) {
            delete editorNode.__agentxSetEditorValue;
            delete editorNode.__agentxGetEditorValue;
          }
        });
      }
      editorInstance.addCommand(monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.KeyS, () => {
        saveFileRef.current();
      });
    },
    [controller]
  );

  return (
    <div
      ref={editorContainerRef}
      className={cn("relative min-h-[18rem] min-w-0 overflow-hidden bg-background", className)}
      data-testid="workspace-file-editor"
      role="region"
      aria-label="File editor"
    >
      {controller.fileLoading && (
        <div className="absolute right-3 top-3 z-10 rounded border border-border bg-background/95 px-2 py-1 text-xs text-muted-foreground shadow-sm">
          Loading...
        </div>
      )}
      <Editor
        height="100%"
        width="100%"
        language={language}
        path={editorModelPath}
        value={controller.fileBody}
        theme={editorTheme}
        options={editorOptions}
        onChange={(value) => controller.setFileBody(value ?? "")}
        onMount={handleEditorMount}
        loading={
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            Loading editor...
          </div>
        }
      />
    </div>
  );
}
