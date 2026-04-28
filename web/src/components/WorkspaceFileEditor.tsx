import { useCallback, useEffect, useMemo, useRef } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import { CircleAlert, RefreshCw } from "lucide-react";
import "@/lib/monaco";
import type { WorkspaceFileEditorProps } from "./WorkspaceFileBrowser";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { monacoLanguageForPath } from "./workspaceFileLanguages";

type WorkspaceEditorNode = HTMLDivElement & {
  __agentxSetEditorValue?: (value: string) => void;
  __agentxGetEditorValue?: () => string;
  __agentxGetEditorPosition?: () => { lineNumber: number; column: number } | null;
};

export function WorkspaceFileEditor({
  controller,
  theme,
  contentAriaLabel,
  className,
}: WorkspaceFileEditorProps) {
  const saveFileRef = useRef<() => void>(() => undefined);
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const lastPositionRequestRef = useRef(0);
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
      editorRef.current = editorInstance;
      editorInstance.onDidDispose(() => {
        if (editorRef.current === editorInstance) {
          editorRef.current = null;
        }
      });
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
          editorNode.__agentxGetEditorPosition = () => editorInstance.getPosition();
        }
        editorInstance.onDidDispose(() => {
          for (const editorNode of editorNodes) {
            delete editorNode.__agentxSetEditorValue;
            delete editorNode.__agentxGetEditorValue;
            delete editorNode.__agentxGetEditorPosition;
          }
        });
      }
      editorInstance.addCommand(monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.KeyS, () => {
        saveFileRef.current();
      });
      if (controller.fileOpenPosition && !controller.fileLoading) {
        requestAnimationFrame(() => {
          if (lastPositionRequestRef.current === controller.fileOpenRequestID) return;
          revealEditorPosition(editorInstance, controller.fileOpenPosition);
          lastPositionRequestRef.current = controller.fileOpenRequestID;
        });
      }
    },
    [controller]
  );

  useEffect(() => {
    if (controller.fileLoading || !controller.fileOpenPosition) return;
    if (lastPositionRequestRef.current === controller.fileOpenRequestID) return;
    const editorInstance = editorRef.current;
    if (!editorInstance) return;
    revealEditorPosition(editorInstance, controller.fileOpenPosition);
    lastPositionRequestRef.current = controller.fileOpenRequestID;
  }, [
    controller.fileLoading,
    controller.fileOpenPosition,
    controller.fileOpenRequestID,
    controller.filePath,
  ]);

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
      {controller.fileLoadError && !controller.fileLoading && (
        <FileLoadErrorOverlay
          path={controller.trimmedPath}
          message={controller.fileLoadError}
          onRetry={() => void controller.loadFile(controller.trimmedPath)}
        />
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

function FileLoadErrorOverlay({
  path,
  message,
  onRetry,
}: {
  path: string;
  message: string;
  onRetry: () => void;
}) {
  return (
    <div
      className="absolute inset-0 z-10 flex items-center justify-center bg-background/95 p-6"
      role="alert"
      data-testid="workspace-file-load-error"
    >
      <div className="max-w-md rounded-md border border-destructive/30 bg-destructive/5 p-4 text-center shadow-sm">
        <CircleAlert className="mx-auto mb-3 h-8 w-8 text-destructive" />
        <h2 className="text-sm font-semibold text-destructive">{fileLoadErrorTitle(message)}</h2>
        {path && (
          <p className="mt-1 break-all text-xs font-medium text-foreground">
            {path}
          </p>
        )}
        <p className="mt-2 text-xs text-muted-foreground">{message}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="mt-4 gap-1.5"
          onClick={onRetry}
        >
          <RefreshCw className="h-3.5 w-3.5" />
          Retry
        </Button>
      </div>
    </div>
  );
}

function fileLoadErrorTitle(message: string): string {
  return /not found/i.test(message) ? "File not found" : "File load failed";
}

function revealEditorPosition(
  editorInstance: editor.IStandaloneCodeEditor,
  position: { lineNumber: number; column?: number } | undefined
) {
  if (!position) return;
  const model = editorInstance.getModel();
  if (!model) return;
  const lineNumber = clamp(position.lineNumber, 1, model.getLineCount());
  const column = clamp(position.column ?? 1, 1, model.getLineMaxColumn(lineNumber));
  const clampedPosition = { lineNumber, column };
  editorInstance.setPosition(clampedPosition);
  editorInstance.revealPositionInCenter(clampedPosition);
  editorInstance.focus();
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
