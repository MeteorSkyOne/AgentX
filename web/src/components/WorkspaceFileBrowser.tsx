import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import { Database, FileText, FolderOpen, RefreshCw, Save, Trash2, X } from "lucide-react";
import "@/lib/monaco";
import type { WorkspaceTreeEntry } from "@/api/types";
import type { ThemeMode } from "@/theme";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";
import { FileTree } from "./FileTree";
import { monacoLanguageForPath } from "./workspaceFileLanguages";

export interface WorkspaceFileBrowserActions {
  onLoadTree: (workspaceID: string) => Promise<WorkspaceTreeEntry>;
  onReadFile: (workspaceID: string, path: string) => Promise<string>;
  onWriteFile: (workspaceID: string, path: string, body: string) => Promise<void>;
  onDeleteFile: (workspaceID: string, path: string) => Promise<void>;
}

interface WorkspaceFileBrowserProps extends WorkspaceFileBrowserActions {
  workspaceID?: string;
  workspacePath?: string;
  initialPath?: string;
  theme: ThemeMode;
}

export interface WorkspaceFileBrowserController {
  workspaceID?: string;
  workspacePath?: string;
  filePath: string;
  fileBody: string;
  tree?: WorkspaceTreeEntry;
  workspaceTreeLoading: boolean;
  workspaceTreeError: string | null;
  fileLoading: boolean;
  fileSaving: boolean;
  fileDeleting: boolean;
  workspaceStatus: string | null;
  trimmedPath: string;
  canUseWorkspace: boolean;
  setFilePath: (path: string) => void;
  setFileBody: (body: string) => void;
  loadTree: (options?: { quiet?: boolean }) => Promise<void>;
  loadFile: (path?: string) => Promise<void>;
  saveFile: () => Promise<void>;
  deleteFile: () => Promise<void>;
}

type WorkspaceEditorNode = HTMLDivElement & {
  __agentxSetEditorValue?: (value: string) => void;
  __agentxGetEditorValue?: () => string;
};

export function useWorkspaceFileBrowser({
  workspaceID,
  workspacePath,
  initialPath = "",
  onLoadTree,
  onReadFile,
  onWriteFile,
  onDeleteFile,
}: Omit<WorkspaceFileBrowserProps, "theme">): WorkspaceFileBrowserController {
  const [filePath, setFilePath] = useState(initialPath);
  const [fileBody, setFileBody] = useState("");
  const [tree, setTree] = useState<WorkspaceTreeEntry>();
  const [workspaceTreeLoading, setWorkspaceTreeLoading] = useState(false);
  const [workspaceTreeError, setWorkspaceTreeError] = useState<string | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileSaving, setFileSaving] = useState(false);
  const [fileDeleting, setFileDeleting] = useState(false);
  const [workspaceStatus, setWorkspaceStatus] = useState<string | null>(null);
  const workspaceTreeRequestRef = useRef(0);
  const fileRequestRef = useRef(0);

  const trimmedPath = filePath.trim();
  const canUseWorkspace = Boolean(workspaceID);

  const loadTree = useCallback(
    async (options: { quiet?: boolean } = {}) => {
      if (!workspaceID) return;
      const requestID = ++workspaceTreeRequestRef.current;
      setWorkspaceTreeLoading(true);
      setWorkspaceTreeError(null);
      try {
        const nextTree = await onLoadTree(workspaceID);
        if (workspaceTreeRequestRef.current !== requestID) return;
        setTree(nextTree);
        if (!options.quiet) setWorkspaceStatus("Tree loaded");
      } catch (err) {
        if (workspaceTreeRequestRef.current !== requestID) return;
        const message = err instanceof Error ? err.message : "Tree load failed";
        setWorkspaceTreeError(message);
        if (!options.quiet) setWorkspaceStatus(message);
      } finally {
        if (workspaceTreeRequestRef.current === requestID) {
          setWorkspaceTreeLoading(false);
        }
      }
    },
    [onLoadTree, workspaceID]
  );

  const loadFile = useCallback(
    async (path = filePath) => {
      const targetPath = path.trim();
      if (!workspaceID || !targetPath) return;
      const requestID = ++fileRequestRef.current;
      setFileLoading(true);
      setFilePath(targetPath);
      try {
        const body = await onReadFile(workspaceID, targetPath);
        if (fileRequestRef.current !== requestID) return;
        setFileBody(body);
        setWorkspaceStatus("Loaded");
      } catch (err) {
        if (fileRequestRef.current !== requestID) return;
        setWorkspaceStatus(err instanceof Error ? err.message : "Load failed");
      } finally {
        if (fileRequestRef.current === requestID) {
          setFileLoading(false);
        }
      }
    },
    [filePath, onReadFile, workspaceID]
  );

  const saveFile = useCallback(async () => {
    const targetPath = filePath.trim();
    if (!workspaceID || !targetPath) return;
    setFileSaving(true);
    try {
      await onWriteFile(workspaceID, targetPath, fileBody);
      setFilePath(targetPath);
      setWorkspaceStatus("Saved");
      await loadTree({ quiet: true });
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Save failed");
    } finally {
      setFileSaving(false);
    }
  }, [fileBody, filePath, loadTree, onWriteFile, workspaceID]);

  const deleteFile = useCallback(async () => {
    const targetPath = filePath.trim();
    if (!workspaceID || !targetPath) return;
    setFileDeleting(true);
    try {
      await onDeleteFile(workspaceID, targetPath);
      setFileBody("");
      setWorkspaceStatus("Deleted");
      await loadTree({ quiet: true });
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setFileDeleting(false);
    }
  }, [filePath, loadTree, onDeleteFile, workspaceID]);

  useEffect(() => {
    workspaceTreeRequestRef.current += 1;
    fileRequestRef.current += 1;
    setTree(undefined);
    setFilePath(initialPath);
    setFileBody("");
    setWorkspaceTreeError(null);
    setWorkspaceTreeLoading(false);
    setFileLoading(false);
    setFileSaving(false);
    setFileDeleting(false);
    setWorkspaceStatus(null);
  }, [initialPath, workspaceID]);

  useEffect(() => {
    if (workspaceID) {
      void loadTree({ quiet: true });
    }
  }, [initialPath, loadTree, workspaceID]);

  return {
    workspaceID,
    workspacePath,
    filePath,
    fileBody,
    tree,
    workspaceTreeLoading,
    workspaceTreeError,
    fileLoading,
    fileSaving,
    fileDeleting,
    workspaceStatus,
    trimmedPath,
    canUseWorkspace,
    setFilePath,
    setFileBody,
    loadTree,
    loadFile,
    saveFile,
    deleteFile,
  };
}

export function WorkspaceFileTreePane({
  controller,
  title = "Project files",
  ariaLabel = "Project files",
  className,
  onFileSelected,
}: {
  controller: WorkspaceFileBrowserController;
  title?: string;
  ariaLabel?: string;
  className?: string;
  onFileSelected?: () => void;
}) {
  return (
    <div className={cn("flex h-full min-h-0 min-w-0 flex-col bg-sidebar", className)} data-testid="project-file-tree-pane">
      <div className="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-border px-4">
        <div className="min-w-0">
          <h2 className="truncate text-sm font-semibold">{title}</h2>
          <p className="truncate text-xs text-muted-foreground">
            {controller.workspacePath || controller.workspaceID || "No workspace"}
          </p>
        </div>
        <Button
          size="icon"
          variant="ghost"
          className="h-8 w-8 shrink-0"
          onClick={() => void controller.loadTree()}
          disabled={controller.workspaceTreeLoading || !controller.canUseWorkspace}
          title="Refresh tree"
          aria-label="Refresh tree"
        >
          <RefreshCw className={cn("h-4 w-4", controller.workspaceTreeLoading && "animate-spin")} />
        </Button>
      </div>
      <div className="min-h-0 flex-1 p-3">
        <WorkspaceFileTree
          controller={controller}
          ariaLabel={ariaLabel}
          className="h-full"
          onFileSelected={onFileSelected}
        />
      </div>
    </div>
  );
}

export function WorkspaceFileTree({
  controller,
  ariaLabel = "Workspace file tree",
  className,
  onFileSelected,
}: {
  controller: WorkspaceFileBrowserController;
  ariaLabel?: string;
  className?: string;
  onFileSelected?: () => void;
}) {
  return (
    <FileTree
      tree={controller.tree}
      selectedPath={controller.filePath}
      loading={controller.workspaceTreeLoading}
      error={controller.workspaceTreeError}
      className={cn("min-h-0", className)}
      ariaLabel={ariaLabel}
      onSelectFile={(path) => {
        void controller.loadFile(path);
        onFileSelected?.();
      }}
    />
  );
}

export function WorkspaceFileEditorPane({
  controller,
  theme,
  title = "Project file",
  contentAriaLabel = "Project file editor",
  className,
  toolbarEnd,
}: {
  controller: WorkspaceFileBrowserController;
  theme: ThemeMode;
  title?: string;
  contentAriaLabel?: string;
  className?: string;
  toolbarEnd?: ReactNode;
}) {
  const [fileDeleteConfirmOpen, setFileDeleteConfirmOpen] = useState(false);

  return (
    <>
      <div className={cn("flex h-full min-h-0 min-w-0 flex-col bg-background", className)} data-testid="project-file-editor-pane">
        <div className="flex shrink-0 flex-col gap-2 border-b border-border p-3">
          <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <Database className="h-3.5 w-3.5 shrink-0" />
            <div className="min-w-0 flex-1">
              <h1 className="truncate text-sm font-semibold text-foreground">
                {controller.filePath || title}
              </h1>
              <p className="truncate">{controller.workspacePath || controller.workspaceID || "No workspace"}</p>
            </div>
            {controller.workspaceStatus && (
              <span className="shrink-0" aria-live="polite">
                {controller.workspaceStatus}
              </span>
            )}
            {toolbarEnd}
          </div>
          <WorkspaceFileToolbar
            controller={controller}
            onDelete={() => setFileDeleteConfirmOpen(true)}
          />
        </div>
        <WorkspaceFileEditor
          controller={controller}
          theme={theme}
          contentAriaLabel={contentAriaLabel}
          className="min-h-0 flex-1"
        />
      </div>
      <WorkspaceFileDeleteDialog
        controller={controller}
        open={fileDeleteConfirmOpen}
        onOpenChange={setFileDeleteConfirmOpen}
      />
    </>
  );
}

function WorkspaceFileToolbar({
  controller,
  onDelete,
  showTreeButton,
  onOpenTree,
}: {
  controller: WorkspaceFileBrowserController;
  onDelete: () => void;
  showTreeButton?: boolean;
  onOpenTree?: () => void;
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      {showTreeButton && (
        <Button
          size="sm"
          variant="outline"
          className="gap-1.5 md:hidden"
          onClick={onOpenTree}
          disabled={!controller.canUseWorkspace}
        >
          <FolderOpen className="h-3.5 w-3.5" />
          File tree
        </Button>
      )}
      <Input
        value={controller.filePath}
        onChange={(event) => controller.setFilePath(event.target.value)}
        aria-label="File path"
        className="h-8 min-w-0 flex-[1_1_12rem] text-xs"
      />
      <Button
        size="sm"
        variant="outline"
        className="gap-1.5"
        onClick={() => void controller.loadFile()}
        disabled={controller.fileLoading || !controller.canUseWorkspace || !controller.trimmedPath}
      >
        <FileText className="h-3.5 w-3.5" />
        Open
      </Button>
      <Button
        size="icon-sm"
        variant="outline"
        onClick={() => void controller.loadTree()}
        disabled={controller.workspaceTreeLoading || !controller.canUseWorkspace}
        title="Refresh tree"
        aria-label="Refresh tree"
      >
        <RefreshCw className={cn("h-3.5 w-3.5", controller.workspaceTreeLoading && "animate-spin")} />
      </Button>
      <Button
        size="sm"
        className="gap-1.5"
        onClick={() => void controller.saveFile()}
        disabled={controller.fileSaving || !controller.canUseWorkspace || !controller.trimmedPath}
      >
        <Save className="h-3.5 w-3.5" />
        Save file
      </Button>
      <Button
        size="icon-sm"
        variant="destructive"
        onClick={onDelete}
        disabled={controller.fileDeleting || !controller.canUseWorkspace || !controller.trimmedPath}
        title="Delete file"
        aria-label="Delete file"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function WorkspaceFileEditor({
  controller,
  theme,
  contentAriaLabel,
  className,
}: {
  controller: WorkspaceFileBrowserController;
  theme: ThemeMode;
  contentAriaLabel: string;
  className?: string;
}) {
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
      wordWrap: "on"
    }),
    [contentAriaLabel, controller.canUseWorkspace, controller.fileLoading]
  );

  useEffect(() => {
    saveFileRef.current = () => {
      void controller.saveFile();
    };
  }, [controller]);

  const handleEditorMount = useCallback<OnMount>((editorInstance, monacoInstance) => {
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
  }, [controller]);

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

function WorkspaceFileDeleteDialog({
  controller,
  open,
  onOpenChange,
}: {
  controller: WorkspaceFileBrowserController;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete file?</DialogTitle>
          <DialogDescription>
            {controller.trimmedPath
              ? `${controller.trimmedPath} will be removed from this workspace.`
              : "This file will be removed from this workspace."}
          </DialogDescription>
        </DialogHeader>
        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={controller.fileDeleting}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={async () => {
              await controller.deleteFile();
              onOpenChange(false);
            }}
            disabled={controller.fileDeleting || !controller.canUseWorkspace || !controller.trimmedPath}
          >
            Delete
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function WorkspaceFileBrowser({
  workspaceID,
  workspacePath,
  initialPath = "",
  theme,
  onLoadTree,
  onReadFile,
  onWriteFile,
  onDeleteFile,
}: WorkspaceFileBrowserProps) {
  const [fileDeleteConfirmOpen, setFileDeleteConfirmOpen] = useState(false);
  const [treeDrawerOpen, setTreeDrawerOpen] = useState(false);
  const [isDesktopLayout, setIsDesktopLayout] = useState(() =>
    typeof window !== "undefined" ? window.matchMedia("(min-width: 768px)").matches : true
  );
  const controller = useWorkspaceFileBrowser({
    workspaceID,
    workspacePath,
    initialPath,
    onLoadTree,
    onReadFile,
    onWriteFile,
    onDeleteFile,
  });

  useEffect(() => {
    const media = window.matchMedia("(min-width: 768px)");
    const update = () => setIsDesktopLayout(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    setFileDeleteConfirmOpen(false);
    setTreeDrawerOpen(false);
  }, [initialPath, workspaceID]);

  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden" aria-label="Workspace files">
        <div className="flex min-h-0 shrink-0 flex-col gap-2">
          <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <Database className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">{workspacePath || workspaceID || "No workspace"}</span>
            {controller.workspaceStatus && (
              <span className="ml-auto shrink-0" aria-live="polite">
                {controller.workspaceStatus}
              </span>
            )}
          </div>

          <WorkspaceFileToolbar
            controller={controller}
            onDelete={() => setFileDeleteConfirmOpen(true)}
            showTreeButton
            onOpenTree={() => setTreeDrawerOpen(true)}
          />
        </div>

        <div className="min-h-0 min-w-0 flex-1 overflow-hidden rounded-md border border-border bg-background">
          {isDesktopLayout ? (
            <ResizablePanelGroup direction="horizontal" className="h-full">
              <ResizablePanel defaultSize={28} minSize={20} maxSize={42}>
                <WorkspaceFileTree controller={controller} className="h-full rounded-none border-0" />
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel defaultSize={72} minSize={48}>
                <WorkspaceFileEditor
                  controller={controller}
                  theme={theme}
                  contentAriaLabel="File content"
                  className="h-full"
                />
              </ResizablePanel>
            </ResizablePanelGroup>
          ) : (
            <WorkspaceFileEditor
              controller={controller}
              theme={theme}
              contentAriaLabel="File content"
              className="h-full"
            />
          )}
        </div>
      </div>

      <Dialog open={treeDrawerOpen} onOpenChange={setTreeDrawerOpen}>
        <DialogContent
          showCloseButton={false}
          className="left-0 top-0 !h-svh !w-[92svw] !max-w-sm !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-l-0 p-0"
        >
          <div className="flex h-full min-h-0 flex-col bg-background">
            <div className="flex h-14 shrink-0 items-center justify-between border-b border-border px-3">
              <DialogHeader className="min-w-0 gap-0 text-left">
                <DialogTitle className="truncate text-base">Workspace file tree</DialogTitle>
                <DialogDescription className="truncate text-xs">
                  {workspacePath || workspaceID || "No workspace"}
                </DialogDescription>
              </DialogHeader>
              <div className="flex items-center gap-1">
                <Button
                  size="icon-sm"
                  variant="ghost"
                  onClick={() => void controller.loadTree()}
                  disabled={controller.workspaceTreeLoading || !controller.canUseWorkspace}
                  title="Refresh tree"
                  aria-label="Refresh tree"
                >
                  <RefreshCw className={cn("h-4 w-4", controller.workspaceTreeLoading && "animate-spin")} />
                </Button>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  onClick={() => setTreeDrawerOpen(false)}
                  title="Close file tree"
                  aria-label="Close file tree"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </div>
            <div className="min-h-0 flex-1 p-3">
              <WorkspaceFileTree
                controller={controller}
                className="h-full"
                onFileSelected={() => setTreeDrawerOpen(false)}
              />
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <WorkspaceFileDeleteDialog
        controller={controller}
        open={fileDeleteConfirmOpen}
        onOpenChange={setFileDeleteConfirmOpen}
      />
    </>
  );
}
