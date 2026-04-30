import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import Editor, { DiffEditor, type DiffOnMount, type OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import { ChevronDown, ChevronUp, CircleAlert, RefreshCw } from "lucide-react";
import "@/lib/monaco";
import type { WorkspaceFileEditorProps } from "./WorkspaceFileBrowser";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import type { WorkspaceGitDiff } from "@/api/types";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { isMarkdownFilePath, monacoLanguageForPath } from "./workspaceFileLanguages";

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
  const isMarkdownFile = isMarkdownFilePath(controller.trimmedPath);
  const viewMode = isMarkdownFile ? controller.fileViewMode : "edit";
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

  const handleOpenPreviewPath = useCallback(
    (target: WorkspacePathTarget) => {
      void controller.loadFile(target.path, {
        position: target.lineNumber
          ? { lineNumber: target.lineNumber, column: target.column ?? 1 }
          : undefined,
      });
    },
    [controller]
  );

  const editorElement = (
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
      {controller.fileLoadError && !controller.fileLoading && (
        <FileLoadErrorOverlay
          path={controller.trimmedPath}
          message={controller.fileLoadError}
          onRetry={() => void controller.loadFile(controller.trimmedPath)}
        />
      )}
      {viewMode === "preview" ? (
        <MarkdownPreview
          controller={controller}
          onOpenWorkspacePath={handleOpenPreviewPath}
        />
      ) : viewMode === "split" ? (
        <div className="flex h-full min-h-0 min-w-0 flex-col md:flex-row">
          <div className="min-h-0 min-w-0 flex-1 border-b border-border md:border-b-0 md:border-r">
            {editorElement}
          </div>
          <MarkdownPreview
            controller={controller}
            onOpenWorkspacePath={handleOpenPreviewPath}
            className="min-h-[12rem] flex-1 md:min-h-0"
          />
        </div>
      ) : (
        editorElement
      )}
    </div>
  );
}

export function WorkspaceGitDiffViewer({
  diff,
  theme,
  contentAriaLabel,
  className,
  navigationContainer,
}: {
  diff: WorkspaceGitDiff;
  theme: "light" | "dark";
  contentAriaLabel: string;
  className?: string;
  navigationContainer?: HTMLElement | null;
}) {
  const editorTheme = theme === "dark" ? "vs-dark" : "light";
  const diffEditorRef = useRef<{
    key: string;
    instance: editor.IStandaloneDiffEditor;
  } | null>(null);
  const [diffNavigationState, setDiffNavigationState] = useState({
    key: "",
    hasChanges: false,
  });
  const language = useMemo(() => monacoLanguageForPath(diff.path), [diff.path]);
  const originalModelPath = useMemo(
    () => gitDiffModelPath("original", diff),
    [diff]
  );
  const modifiedModelPath = useMemo(
    () => gitDiffModelPath("modified", diff),
    [diff]
  );
  const diffEditorKey = `${originalModelPath}\n${modifiedModelPath}`;
  const hasDiffChanges =
    diffNavigationState.key === diffEditorKey && diffNavigationState.hasChanges;
  const options = useMemo<editor.IDiffEditorConstructionOptions>(
    () => ({
      ariaLabel: contentAriaLabel,
      automaticLayout: true,
      fontSize: 13,
      hideUnchangedRegions: {
        enabled: true,
        contextLineCount: 3,
        minimumLineCount: 8,
        revealLineCount: 20,
      },
      minimap: { enabled: false },
      originalEditable: false,
      readOnly: true,
      diffWordWrap: "off",
      renderSideBySide: true,
      scrollBeyondLastLine: false,
      wordWrap: "off",
    }),
    [contentAriaLabel]
  );
  const handleDiffEditorMount = useCallback<DiffOnMount>((editorInstance) => {
    const mountedDiffKey = diffEditorKey;
    diffEditorRef.current = { key: mountedDiffKey, instance: editorInstance };

    const updateChanges = () => {
      setDiffNavigationState({
        key: mountedDiffKey,
        hasChanges: (editorInstance.getLineChanges()?.length ?? 0) > 0,
      });
    };
    updateChanges();
    const diffSubscription = editorInstance.onDidUpdateDiff(updateChanges);
    const disposeSubscription = editorInstance.onDidDispose(() => {
      diffSubscription.dispose();
      disposeSubscription.dispose();
      if (diffEditorRef.current?.instance === editorInstance) {
        diffEditorRef.current = null;
      }
    });
  }, [diffEditorKey]);
  const navigateDiff = useCallback((target: "previous" | "next") => {
    const currentEditor = diffEditorRef.current;
    if (currentEditor?.key !== diffEditorKey) return;
    currentEditor.instance.goToDiff(target);
  }, [diffEditorKey]);
  const navigationControls = (
    <div
      className="flex overflow-hidden rounded-md border border-border bg-background shadow-sm"
      data-testid="workspace-git-diff-navigation"
    >
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        title="Previous change"
        aria-label="Previous change"
        disabled={!hasDiffChanges}
        className="rounded-none"
        onClick={() => navigateDiff("previous")}
      >
        <ChevronUp className="h-3.5 w-3.5" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        title="Next change"
        aria-label="Next change"
        disabled={!hasDiffChanges}
        className="rounded-none border-l border-border"
        onClick={() => navigateDiff("next")}
      >
        <ChevronDown className="h-3.5 w-3.5" />
      </Button>
    </div>
  );

  return (
    <div
      className={cn("min-h-[18rem] min-w-0 overflow-hidden bg-background", className)}
      data-testid="workspace-git-diff-viewer"
      role="region"
      aria-label="Git diff preview"
    >
      {navigationContainer ? createPortal(navigationControls, navigationContainer) : null}
      <DiffEditor
        key={diffEditorKey}
        height="100%"
        width="100%"
        language={language}
        original={diff.original}
        modified={diff.modified}
        originalModelPath={originalModelPath}
        modifiedModelPath={modifiedModelPath}
        theme={editorTheme}
        options={options}
        onMount={handleDiffEditorMount}
        loading={
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            Loading diff editor...
          </div>
        }
      />
    </div>
  );
}

function MarkdownPreview({
  controller,
  onOpenWorkspacePath,
  className,
}: {
  controller: WorkspaceFileEditorProps["controller"];
  onOpenWorkspacePath: (target: WorkspacePathTarget) => void;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "prose prose-sm h-full min-h-0 w-full max-w-none overflow-auto break-words bg-background p-4 text-foreground dark:prose-invert",
        className
      )}
      data-testid="workspace-file-markdown-preview"
    >
      <MarkdownRenderer
        text={controller.fileBody}
        workspacePath={controller.workspacePath}
        relativeLinkBasePath={controller.trimmedPath}
        onOpenWorkspacePath={onOpenWorkspacePath}
      />
    </div>
  );
}

function gitDiffModelPath(side: "original" | "modified", diff: WorkspaceGitDiff): string {
  const path = side === "original" ? diff.old_path || diff.path : diff.path;
  const encodedPath = path.split("/").map(encodeURIComponent).join("/");
  const scope = encodeURIComponent(diff.scope);
  const ref = encodeURIComponent(side === "original" ? diff.target || diff.base || "HEAD" : diff.compare || diff.branch || "workspace");
  return `agentx://git-diff/${scope}/${ref}/${side}/${encodedPath}`;
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
