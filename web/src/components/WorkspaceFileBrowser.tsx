import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import {
  ChevronDown,
  ChevronUp,
  Code2,
  Columns2,
  Database,
  Eye,
  FileText,
  FolderOpen,
  RefreshCw,
  Save,
  Trash2,
  X,
} from "lucide-react";
import type { WorkspaceEntryType, WorkspaceTreeEntry } from "@/api/types";
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
import { isMarkdownFilePath } from "./workspaceFileLanguages";

const LazyWorkspaceFileEditor = lazy(() =>
  import("./WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceFileEditor }))
);

export interface WorkspaceFileBrowserActions {
  onLoadTree: (workspaceID: string, path?: string) => Promise<WorkspaceTreeEntry>;
  onReadFile: (workspaceID: string, path: string) => Promise<string>;
  onWriteFile: (workspaceID: string, path: string, body: string) => Promise<void>;
  onDeleteFile: (workspaceID: string, path: string) => Promise<void>;
  onCreateEntry?: (workspaceID: string, path: string, type: WorkspaceEntryType) => Promise<void>;
  onMoveEntry?: (workspaceID: string, path: string, newPath: string) => Promise<void>;
  onDeleteEntry?: (workspaceID: string, path: string) => Promise<void>;
}

export interface WorkspaceFilePosition {
  lineNumber: number;
  column?: number;
}

export interface WorkspaceFileOpenOptions {
  position?: WorkspaceFilePosition;
}

export type WorkspaceFileViewMode = "edit" | "preview" | "split";

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
  workspaceTreeResetKey: number;
  workspaceTreeLoading: boolean;
  workspaceTreeError: string | null;
  directoryLoadingPaths: ReadonlySet<string>;
  directoryLoadErrors: Record<string, string | null | undefined>;
  fileLoading: boolean;
  fileLoadError: string | null;
  fileSaving: boolean;
  fileDeleting: boolean;
  entryActionPending: boolean;
  workspaceStatus: string | null;
  fileOpenPosition?: WorkspaceFilePosition;
  fileOpenRequestID: number;
  fileViewMode: WorkspaceFileViewMode;
  trimmedPath: string;
  canUseWorkspace: boolean;
  setFilePath: (path: string) => void;
  setFileBody: (body: string) => void;
  setFileViewMode: (mode: WorkspaceFileViewMode) => void;
  loadTree: (options?: { quiet?: boolean }) => Promise<void>;
  loadDirectory: (path: string, options?: { quiet?: boolean; force?: boolean }) => Promise<void>;
  loadFile: (path?: string, options?: WorkspaceFileOpenOptions) => Promise<void>;
  saveFile: () => Promise<void>;
  deleteFile: () => Promise<void>;
  createEntry: (
    parentPath: string,
    name: string,
    type: WorkspaceEntryType
  ) => Promise<string | null>;
  renameEntry: (entry: WorkspaceTreeEntry, name: string) => Promise<string | null>;
  deleteEntry: (entry: WorkspaceTreeEntry) => Promise<void>;
  moveEntry: (entry: WorkspaceTreeEntry, targetDirectoryPath: string) => Promise<string | null>;
}

export function useWorkspaceFileBrowser({
  workspaceID,
  workspacePath,
  initialPath = "",
  onLoadTree,
  onReadFile,
  onWriteFile,
  onDeleteFile,
  onCreateEntry,
  onMoveEntry,
  onDeleteEntry,
}: Omit<WorkspaceFileBrowserProps, "theme">): WorkspaceFileBrowserController {
  const [filePath, setFilePath] = useState(initialPath);
  const [fileBody, setFileBody] = useState("");
  const [tree, setTree] = useState<WorkspaceTreeEntry>();
  const [workspaceTreeResetKey, setWorkspaceTreeResetKey] = useState(0);
  const [workspaceTreeLoading, setWorkspaceTreeLoading] = useState(false);
  const [workspaceTreeError, setWorkspaceTreeError] = useState<string | null>(null);
  const [directoryLoadingPaths, setDirectoryLoadingPaths] = useState<Set<string>>(() => new Set());
  const [directoryLoadErrors, setDirectoryLoadErrors] = useState<Record<string, string | null | undefined>>({});
  const [fileLoading, setFileLoading] = useState(false);
  const [fileLoadError, setFileLoadError] = useState<string | null>(null);
  const [fileSaving, setFileSaving] = useState(false);
  const [fileDeleting, setFileDeleting] = useState(false);
  const [entryActionPending, setEntryActionPending] = useState(false);
  const [workspaceStatus, setWorkspaceStatus] = useState<string | null>(null);
  const [fileOpenPosition, setFileOpenPosition] = useState<WorkspaceFilePosition>();
  const [fileOpenRequestID, setFileOpenRequestID] = useState(0);
  const [fileViewMode, setFileViewMode] = useState<WorkspaceFileViewMode>("edit");
  const workspaceTreeRequestRef = useRef(0);
  const directoryRequestRef = useRef<Record<string, number>>({});
  const fileRequestRef = useRef(0);

  const trimmedPath = filePath.trim();
  const canUseWorkspace = Boolean(workspaceID);

  const loadTree = useCallback(
    async (options: { quiet?: boolean } = {}) => {
      if (!workspaceID) return;
      const requestID = ++workspaceTreeRequestRef.current;
      directoryRequestRef.current = {};
      setWorkspaceTreeLoading(true);
      setWorkspaceTreeError(null);
      setDirectoryLoadingPaths(new Set());
      setDirectoryLoadErrors({});
      try {
        const nextTree = await onLoadTree(workspaceID);
        if (workspaceTreeRequestRef.current !== requestID) return;
        setTree(nextTree);
        setWorkspaceTreeResetKey((current) => current + 1);
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

  const loadDirectory = useCallback(
    async (path: string, options: { quiet?: boolean; force?: boolean } = {}) => {
      const targetPath = normalizeWorkspaceTreePath(path);
      if (!workspaceID || !targetPath) return;
      if (!options.force && workspaceTreeDirectoryLoaded(tree, targetPath)) return;

      const requestID = (directoryRequestRef.current[targetPath] ?? 0) + 1;
      const treeRequestID = workspaceTreeRequestRef.current;
      directoryRequestRef.current[targetPath] = requestID;
      setDirectoryLoadingPaths((current) => new Set(current).add(targetPath));
      setDirectoryLoadErrors((current) => {
        const next = { ...current };
        delete next[targetPath];
        return next;
      });

      try {
        const nextTree = await onLoadTree(workspaceID, targetPath);
        if (
          workspaceTreeRequestRef.current !== treeRequestID ||
          directoryRequestRef.current[targetPath] !== requestID
        ) {
          return;
        }
        setTree((current) => mergeWorkspaceTreeEntry(current, nextTree));
        if (!options.quiet) setWorkspaceStatus("Tree loaded");
      } catch (err) {
        if (
          workspaceTreeRequestRef.current !== treeRequestID ||
          directoryRequestRef.current[targetPath] !== requestID
        ) {
          return;
        }
        const message = err instanceof Error ? err.message : "Directory load failed";
        setDirectoryLoadErrors((current) => ({ ...current, [targetPath]: message }));
        if (!options.quiet) setWorkspaceStatus(message);
      } finally {
        if (directoryRequestRef.current[targetPath] === requestID) {
          setDirectoryLoadingPaths((current) => {
            const next = new Set(current);
            next.delete(targetPath);
            return next;
          });
        }
      }
    },
    [onLoadTree, tree, workspaceID]
  );

  const refreshTreeForFilePath = useCallback(
    async (path: string) => {
      const parentPath = parentWorkspaceDirectoryPath(path);
      if (!parentPath) {
        await loadTree({ quiet: true });
        return;
      }
      if (workspaceTreeDirectoryLoaded(tree, parentPath)) {
        await loadDirectory(parentPath, { quiet: true, force: true });
      }
    },
    [loadDirectory, loadTree, tree]
  );

  const refreshTreeForEntryPaths = useCallback(
    async (paths: string[]) => {
      const parentPaths = new Set(
        paths.map((path) => parentWorkspaceDirectoryPath(path))
      );
      if (parentPaths.has("")) {
        await loadTree({ quiet: true });
        parentPaths.delete("");
      }
      for (const parentPath of parentPaths) {
        if (workspaceTreeDirectoryLoaded(tree, parentPath)) {
          await loadDirectory(parentPath, { quiet: true, force: true });
        }
      }
    },
    [loadDirectory, loadTree, tree]
  );

  const loadFile = useCallback(
    async (path = filePath, options: WorkspaceFileOpenOptions = {}) => {
      const targetPath = path.trim();
      if (!workspaceID || !targetPath) return;
      const requestID = ++fileRequestRef.current;
      setFileLoading(true);
      setFileLoadError(null);
      setFilePath(targetPath);
      setFileOpenPosition(normalizeWorkspaceFilePosition(options.position));
      setFileOpenRequestID(requestID);
      try {
        const body = await onReadFile(workspaceID, targetPath);
        if (fileRequestRef.current !== requestID) return;
        setFileBody(body);
        setFileLoadError(null);
        setWorkspaceStatus("Loaded");
      } catch (err) {
        if (fileRequestRef.current !== requestID) return;
        const message = err instanceof Error ? err.message : "Load failed";
        setFileBody("");
        setFileLoadError(message);
        setWorkspaceStatus(message);
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
      setFileLoadError(null);
      setWorkspaceStatus("Saved");
      await refreshTreeForFilePath(targetPath);
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Save failed");
    } finally {
      setFileSaving(false);
    }
  }, [fileBody, filePath, onWriteFile, refreshTreeForFilePath, workspaceID]);

  const deleteFile = useCallback(async () => {
    const targetPath = filePath.trim();
    if (!workspaceID || !targetPath) return;
    setFileDeleting(true);
    try {
      await onDeleteFile(workspaceID, targetPath);
      setFileBody("");
      setFileLoadError(null);
      setWorkspaceStatus("Deleted");
      await refreshTreeForFilePath(targetPath);
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setFileDeleting(false);
    }
  }, [filePath, onDeleteFile, refreshTreeForFilePath, workspaceID]);

  const createEntry = useCallback(
    async (parentPath: string, name: string, type: WorkspaceEntryType) => {
      if (!workspaceID) return null;
      const entryName = normalizeWorkspaceEntryName(name);
      const targetPath = joinWorkspacePath(parentPath, entryName);
      setEntryActionPending(true);
      try {
        if (onCreateEntry) {
          await onCreateEntry(workspaceID, targetPath, type);
        } else if (type === "file") {
          await onWriteFile(workspaceID, targetPath, "");
        } else {
          throw new Error("Folder creation is not supported");
        }
        await refreshTreeForEntryPaths([targetPath]);
        if (type === "file") {
          fileRequestRef.current += 1;
          setFilePath(targetPath);
          setFileBody("");
          setFileLoadError(null);
          setFileOpenPosition(undefined);
          setFileOpenRequestID(fileRequestRef.current);
        }
        setWorkspaceStatus(type === "directory" ? "Folder created" : "File created");
        return targetPath;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Create failed";
        setWorkspaceStatus(message);
        throw err;
      } finally {
        setEntryActionPending(false);
      }
    },
    [onCreateEntry, onWriteFile, refreshTreeForEntryPaths, workspaceID]
  );

  const renameEntry = useCallback(
    async (entry: WorkspaceTreeEntry, name: string) => {
      if (!workspaceID) return null;
      const entryName = normalizeWorkspaceEntryName(name);
      const targetPath = joinWorkspacePath(parentWorkspaceDirectoryPath(entry.path), entryName);
      if (targetPath === entry.path) return targetPath;
      if (!onMoveEntry) {
        const message = "Rename is not supported";
        setWorkspaceStatus(message);
        throw new Error(message);
      }
      setEntryActionPending(true);
      try {
        await onMoveEntry(workspaceID, entry.path, targetPath);
        setFilePath((currentPath) => replaceMovedWorkspacePath(currentPath, entry.path, targetPath));
        setWorkspaceStatus("Renamed");
        await refreshTreeForEntryPaths([entry.path, targetPath]);
        return targetPath;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Rename failed";
        setWorkspaceStatus(message);
        throw err;
      } finally {
        setEntryActionPending(false);
      }
    },
    [onMoveEntry, refreshTreeForEntryPaths, workspaceID]
  );

  const deleteEntry = useCallback(
    async (entry: WorkspaceTreeEntry) => {
      if (!workspaceID) return;
      setEntryActionPending(true);
      if (entry.path === filePath.trim()) {
        setFileDeleting(true);
      }
      try {
        if (onDeleteEntry) {
          await onDeleteEntry(workspaceID, entry.path);
        } else if (entry.type === "file") {
          await onDeleteFile(workspaceID, entry.path);
        } else {
          throw new Error("Folder deletion is not supported");
        }
        const openPathAffected = workspacePathIsSameOrInside(filePath.trim(), entry.path);
        setFilePath((currentPath) =>
          workspacePathIsSameOrInside(currentPath.trim(), entry.path) ? "" : currentPath
        );
        if (openPathAffected) {
          setFileBody("");
          setFileLoadError(null);
        }
        setWorkspaceStatus("Deleted");
        await refreshTreeForEntryPaths([entry.path]);
      } catch (err) {
        const message = err instanceof Error ? err.message : "Delete failed";
        setWorkspaceStatus(message);
        throw err;
      } finally {
        setFileDeleting(false);
        setEntryActionPending(false);
      }
    },
    [filePath, onDeleteEntry, onDeleteFile, refreshTreeForEntryPaths, workspaceID]
  );

  const moveEntry = useCallback(
    async (entry: WorkspaceTreeEntry, targetDirectoryPath: string) => {
      if (!workspaceID) return null;
      const targetDirectory = normalizeWorkspaceTreePath(targetDirectoryPath);
      if (entry.type === "directory" && workspacePathIsSameOrInside(targetDirectory, entry.path)) {
        const message = "A folder cannot be moved into itself";
        setWorkspaceStatus(message);
        throw new Error(message);
      }
      const targetPath = joinWorkspacePath(targetDirectory, entry.name);
      if (targetPath === entry.path) return targetPath;
      if (!onMoveEntry) {
        const message = "Move is not supported";
        setWorkspaceStatus(message);
        throw new Error(message);
      }
      setEntryActionPending(true);
      try {
        await onMoveEntry(workspaceID, entry.path, targetPath);
        setFilePath((currentPath) => replaceMovedWorkspacePath(currentPath, entry.path, targetPath));
        setWorkspaceStatus("Moved");
        await refreshTreeForEntryPaths([entry.path, targetPath]);
        return targetPath;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Move failed";
        setWorkspaceStatus(message);
        throw err;
      } finally {
        setEntryActionPending(false);
      }
    },
    [onMoveEntry, refreshTreeForEntryPaths, workspaceID]
  );

  useEffect(() => {
    workspaceTreeRequestRef.current += 1;
    directoryRequestRef.current = {};
    fileRequestRef.current += 1;
    setTree(undefined);
    setWorkspaceTreeResetKey((current) => current + 1);
    setFilePath(initialPath);
    setFileBody("");
    setWorkspaceTreeError(null);
    setDirectoryLoadingPaths(new Set());
    setDirectoryLoadErrors({});
    setFileLoadError(null);
    setWorkspaceTreeLoading(false);
    setFileLoading(false);
    setFileSaving(false);
    setFileDeleting(false);
    setEntryActionPending(false);
    setFileOpenPosition(undefined);
    setFileOpenRequestID(0);
    setFileViewMode("edit");
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
    workspaceTreeResetKey,
    workspaceTreeLoading,
    workspaceTreeError,
    directoryLoadingPaths,
    directoryLoadErrors,
    fileLoading,
    fileLoadError,
    fileSaving,
    fileDeleting,
    entryActionPending,
    workspaceStatus,
    fileOpenPosition,
    fileOpenRequestID,
    fileViewMode,
    trimmedPath,
    canUseWorkspace,
    setFilePath,
    setFileBody,
    setFileViewMode,
    loadTree,
    loadDirectory,
    loadFile,
    saveFile,
    deleteFile,
    createEntry,
    renameEntry,
    deleteEntry,
    moveEntry,
  };
}

function normalizeWorkspaceFilePosition(
  position?: WorkspaceFilePosition
): WorkspaceFilePosition | undefined {
  if (!position || position.lineNumber < 1) return undefined;
  return {
    lineNumber: position.lineNumber,
    column: position.column && position.column > 0 ? position.column : 1,
  };
}

function normalizeWorkspaceTreePath(path: string): string {
  const normalized = path.trim().replaceAll("\\", "/").replace(/^\/+|\/+$/g, "");
  return normalized === "." ? "" : normalized;
}

function normalizeWorkspaceEntryName(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) {
    throw new Error("Name is required");
  }
  if (
    trimmed === "." ||
    trimmed === ".." ||
    trimmed.includes("/") ||
    trimmed.includes("\\")
  ) {
    throw new Error("Use a single file or folder name");
  }
  return trimmed;
}

function joinWorkspacePath(parentPath: string, name: string): string {
  const normalizedParent = normalizeWorkspaceTreePath(parentPath);
  return normalizedParent ? `${normalizedParent}/${name}` : name;
}

function workspacePathIsSameOrInside(path: string, parentPath: string): boolean {
  const normalizedPath = normalizeWorkspaceTreePath(path);
  const normalizedParent = normalizeWorkspaceTreePath(parentPath);
  if (!normalizedPath || !normalizedParent) return normalizedPath === normalizedParent;
  return normalizedPath === normalizedParent || normalizedPath.startsWith(`${normalizedParent}/`);
}

function replaceMovedWorkspacePath(path: string, sourcePath: string, targetPath: string): string {
  const normalizedPath = normalizeWorkspaceTreePath(path);
  const normalizedSource = normalizeWorkspaceTreePath(sourcePath);
  const normalizedTarget = normalizeWorkspaceTreePath(targetPath);
  if (!workspacePathIsSameOrInside(normalizedPath, normalizedSource)) return path;
  return `${normalizedTarget}${normalizedPath.slice(normalizedSource.length)}`;
}

function parentWorkspaceDirectoryPath(path: string): string {
  const normalized = normalizeWorkspaceTreePath(path);
  const slashIndex = normalized.lastIndexOf("/");
  return slashIndex === -1 ? "" : normalized.slice(0, slashIndex);
}

function workspaceTreeDirectoryLoaded(tree: WorkspaceTreeEntry | undefined, path: string): boolean {
  const entry = findWorkspaceTreeEntry(tree, path);
  return entry?.type === "directory" && Boolean(entry.children_loaded);
}

function findWorkspaceTreeEntry(
  entry: WorkspaceTreeEntry | undefined,
  path: string
): WorkspaceTreeEntry | undefined {
  if (!entry) return undefined;
  if (entry.path === path) return entry;
  for (const child of entry.children ?? []) {
    if (child.path === path || path.startsWith(`${child.path}/`)) {
      const match = findWorkspaceTreeEntry(child, path);
      if (match) return match;
    }
  }
  return undefined;
}

function uniqueWorkspaceChildName(
  tree: WorkspaceTreeEntry | undefined,
  parentPath: string,
  baseName: string
): string {
  const parent = findWorkspaceTreeEntry(tree, normalizeWorkspaceTreePath(parentPath));
  const names = new Set((parent?.children ?? []).map((child) => child.name));
  if (!names.has(baseName)) return baseName;

  const dotIndex = baseName.lastIndexOf(".");
  const hasExtension = dotIndex > 0 && dotIndex < baseName.length - 1;
  const stem = hasExtension ? baseName.slice(0, dotIndex) : baseName;
  const extension = hasExtension ? baseName.slice(dotIndex) : "";
  for (let index = 2; index < 1000; index += 1) {
    const candidate = `${stem}-${index}${extension}`;
    if (!names.has(candidate)) return candidate;
  }
  return `${stem}-${Date.now()}${extension}`;
}

function workspaceEntryNameValidationError(name: string): string | null {
  const trimmed = name.trim();
  if (!trimmed) return "Name is required";
  if (
    trimmed === "." ||
    trimmed === ".." ||
    trimmed.includes("/") ||
    trimmed.includes("\\")
  ) {
    return "Use a single file or folder name";
  }
  return null;
}

function mergeWorkspaceTreeEntry(
  current: WorkspaceTreeEntry | undefined,
  nextEntry: WorkspaceTreeEntry
): WorkspaceTreeEntry {
  if (!current || current.path === nextEntry.path) return nextEntry;
  if (current.type !== "directory" || !current.children?.length) return current;

  let changed = false;
  const children = current.children.map((child) => {
    if (child.path === nextEntry.path) {
      changed = true;
      return nextEntry;
    }
    if (nextEntry.path.startsWith(`${child.path}/`)) {
      const merged = mergeWorkspaceTreeEntry(child, nextEntry);
      if (merged !== child) changed = true;
      return merged;
    }
    return child;
  });

  return changed ? { ...current, children } : current;
}

export function WorkspaceFileTreePane({
  controller,
  title = "Project files",
  ariaLabel = "Project files",
  className,
  onFileSelected,
  toolbarEnd,
}: {
  controller: WorkspaceFileBrowserController;
  title?: string;
  ariaLabel?: string;
  className?: string;
  onFileSelected?: () => void;
  toolbarEnd?: ReactNode;
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
        <div className="flex shrink-0 items-center gap-1">
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
          {toolbarEnd}
        </div>
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
  const [entryDialog, setEntryDialog] = useState<WorkspaceEntryDialog | null>(null);
  const [entryName, setEntryName] = useState("");
  const [entryDialogError, setEntryDialogError] = useState<string | null>(null);

  const nameDialog = entryDialog?.kind === "create" || entryDialog?.kind === "rename"
    ? entryDialog
    : null;
  const deleteDialog = entryDialog?.kind === "delete" ? entryDialog : null;

  function openCreateDialog(parentPath: string, type: WorkspaceEntryType) {
    setEntryDialog({ kind: "create", parentPath, entryType: type });
    setEntryName(uniqueWorkspaceChildName(
      controller.tree,
      parentPath,
      type === "directory" ? "new-folder" : "untitled.txt"
    ));
    setEntryDialogError(null);
  }

  function openRenameDialog(entry: WorkspaceTreeEntry) {
    setEntryDialog({ kind: "rename", entry });
    setEntryName(entry.name);
    setEntryDialogError(null);
  }

  function openDeleteDialog(entry: WorkspaceTreeEntry) {
    setEntryDialog({ kind: "delete", entry });
    setEntryDialogError(null);
  }

  return (
    <>
      <FileTree
        tree={controller.tree}
        selectedPath={controller.filePath}
        loading={controller.workspaceTreeLoading}
        error={controller.workspaceTreeError}
        className={cn("min-h-0", className)}
        ariaLabel={ariaLabel}
        resetKey={controller.workspaceTreeResetKey}
        directoryLoadingPaths={controller.directoryLoadingPaths}
        directoryErrors={controller.directoryLoadErrors}
        canManageEntries={controller.canUseWorkspace && !controller.entryActionPending}
        onLoadDirectory={(path) => void controller.loadDirectory(path)}
        onSelectFile={(path) => {
          void controller.loadFile(path);
          onFileSelected?.();
        }}
        onCreateEntry={openCreateDialog}
        onRenameEntry={openRenameDialog}
        onDeleteEntry={openDeleteDialog}
        onMoveEntry={(entry, targetDirectoryPath) => {
          void controller.moveEntry(entry, targetDirectoryPath).catch(() => undefined);
        }}
      />
      <WorkspaceEntryNameDialog
        dialog={nameDialog}
        name={entryName}
        error={entryDialogError}
        pending={controller.entryActionPending}
        onNameChange={(name) => {
          setEntryName(name);
          setEntryDialogError(null);
        }}
        onOpenChange={(open) => {
          if (!open) setEntryDialog(null);
        }}
        onSubmit={async () => {
          const validationError = workspaceEntryNameValidationError(entryName);
          if (validationError) {
            setEntryDialogError(validationError);
            return;
          }
          if (!nameDialog) return;
          try {
            if (nameDialog.kind === "create") {
              await controller.createEntry(
                nameDialog.parentPath,
                entryName,
                nameDialog.entryType
              );
            } else {
              await controller.renameEntry(nameDialog.entry, entryName);
            }
            setEntryDialog(null);
          } catch (err) {
            setEntryDialogError(err instanceof Error ? err.message : "Action failed");
          }
        }}
      />
      <WorkspaceEntryDeleteDialog
        dialog={deleteDialog}
        error={entryDialogError}
        pending={controller.entryActionPending}
        onOpenChange={(open) => {
          if (!open) setEntryDialog(null);
        }}
        onDelete={async () => {
          if (!deleteDialog) return;
          try {
            await controller.deleteEntry(deleteDialog.entry);
            setEntryDialog(null);
          } catch (err) {
            setEntryDialogError(err instanceof Error ? err.message : "Delete failed");
          }
        }}
      />
    </>
  );
}

type WorkspaceEntryDialog =
  | { kind: "create"; parentPath: string; entryType: WorkspaceEntryType }
  | { kind: "rename"; entry: WorkspaceTreeEntry }
  | { kind: "delete"; entry: WorkspaceTreeEntry };

function WorkspaceEntryNameDialog({
  dialog,
  name,
  error,
  pending,
  onNameChange,
  onOpenChange,
  onSubmit,
}: {
  dialog: Extract<WorkspaceEntryDialog, { kind: "create" | "rename" }> | null;
  name: string;
  error: string | null;
  pending: boolean;
  onNameChange: (name: string) => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: () => Promise<void>;
}) {
  const isCreate = dialog?.kind === "create";
  const entryType = isCreate ? dialog.entryType : dialog?.entry.type;
  const title = isCreate
    ? `New ${entryType === "directory" ? "folder" : "file"}`
    : `Rename ${entryType === "directory" ? "folder" : "file"}`;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await onSubmit();
  }

  return (
    <Dialog open={Boolean(dialog)} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {isCreate
              ? "Create an entry in this workspace."
              : "Set a new name for this workspace entry."}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-3" onSubmit={handleSubmit}>
          <Input
            value={name}
            onChange={(event) => onNameChange(event.target.value)}
            aria-label="Entry name"
            autoFocus
          />
          {error && <p className="text-xs text-destructive">{error}</p>}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={pending || !name.trim()}>
              {isCreate ? "Create" : "Rename"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function WorkspaceEntryDeleteDialog({
  dialog,
  error,
  pending,
  onOpenChange,
  onDelete,
}: {
  dialog: Extract<WorkspaceEntryDialog, { kind: "delete" }> | null;
  error: string | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => Promise<void>;
}) {
  const entry = dialog?.entry;

  return (
    <Dialog open={Boolean(dialog)} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete {entry?.type === "directory" ? "folder" : "file"}?</DialogTitle>
          <DialogDescription>
            {entry
              ? entry.type === "directory"
                ? `${entry.path} and its contents will be removed from this workspace.`
                : `${entry.path} will be removed from this workspace.`
              : "This workspace entry will be removed."}
          </DialogDescription>
        </DialogHeader>
        {error && <p className="text-xs text-destructive">{error}</p>}
        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => void onDelete()}
            disabled={pending || !entry}
          >
            Delete
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function WorkspaceFileEditorPane({
  controller,
  theme,
  title = "Project file",
  contentAriaLabel = "Project file editor",
  className,
  toolbarEnd,
  headerCollapsed: controlledHeaderCollapsed,
  onHeaderCollapsedChange,
  headerControlsPlacement = "pane",
}: {
  controller: WorkspaceFileBrowserController;
  theme: ThemeMode;
  title?: string;
  contentAriaLabel?: string;
  className?: string;
  toolbarEnd?: ReactNode;
  headerCollapsed?: boolean;
  onHeaderCollapsedChange?: (collapsed: boolean) => void;
  headerControlsPlacement?: "pane" | "external";
}) {
  const [fileDeleteConfirmOpen, setFileDeleteConfirmOpen] = useState(false);
  const [internalHeaderCollapsed, setInternalHeaderCollapsed] = useState(false);
  const headerCollapsed = controlledHeaderCollapsed ?? internalHeaderCollapsed;
  const collapseControlsInPane = headerControlsPlacement === "pane";

  function setHeaderCollapsed(collapsed: boolean) {
    if (onHeaderCollapsedChange) {
      onHeaderCollapsedChange(collapsed);
    } else {
      setInternalHeaderCollapsed(collapsed);
    }
  }

  return (
    <>
      <div className={cn("flex h-full min-h-0 min-w-0 flex-col bg-background", className)} data-testid="project-file-editor-pane">
        {headerCollapsed && collapseControlsInPane ? (
          <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
            <span className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">
              {controller.filePath || title}
            </span>
            <div className="flex shrink-0 items-center gap-1">
              {toolbarEnd}
            </div>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              title="Show file path bar"
              aria-label="Show file path bar"
              aria-expanded="false"
              onClick={() => setHeaderCollapsed(false)}
            >
              <ChevronDown className="h-4 w-4" />
            </Button>
          </div>
        ) : !headerCollapsed ? (
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
              {collapseControlsInPane && (
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  title="Hide file path bar"
                  aria-label="Hide file path bar"
                  aria-expanded="true"
                  onClick={() => setHeaderCollapsed(true)}
                >
                  <ChevronUp className="h-4 w-4" />
                </Button>
              )}
            </div>
            <WorkspaceFileToolbar
              controller={controller}
              onDelete={() => setFileDeleteConfirmOpen(true)}
            />
          </div>
        ) : (
          null
        )}
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
  const markdownControlsVisible = isMarkdownFilePath(controller.trimmedPath);

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
      {markdownControlsVisible && (
        <div
          className="flex shrink-0 items-center rounded-md border border-border bg-background p-0.5"
          role="group"
          aria-label="Markdown view"
        >
          <MarkdownViewButton
            mode="edit"
            currentMode={controller.fileViewMode}
            label="Edit Markdown"
            onSelect={controller.setFileViewMode}
          >
            <Code2 className="h-3.5 w-3.5" />
          </MarkdownViewButton>
          <MarkdownViewButton
            mode="preview"
            currentMode={controller.fileViewMode}
            label="Preview Markdown"
            onSelect={controller.setFileViewMode}
          >
            <Eye className="h-3.5 w-3.5" />
          </MarkdownViewButton>
          <MarkdownViewButton
            mode="split"
            currentMode={controller.fileViewMode}
            label="Split Markdown view"
            onSelect={controller.setFileViewMode}
          >
            <Columns2 className="h-3.5 w-3.5" />
          </MarkdownViewButton>
        </div>
      )}
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

function MarkdownViewButton({
  mode,
  currentMode,
  label,
  onSelect,
  children,
}: {
  mode: WorkspaceFileViewMode;
  currentMode: WorkspaceFileViewMode;
  label: string;
  onSelect: (mode: WorkspaceFileViewMode) => void;
  children: ReactNode;
}) {
  const selected = mode === currentMode;
  return (
    <Button
      type="button"
      size="icon-sm"
      variant={selected ? "secondary" : "ghost"}
      className="h-7 w-7"
      title={label}
      aria-label={label}
      aria-pressed={selected}
      onClick={() => onSelect(mode)}
    >
      {children}
    </Button>
  );
}

export interface WorkspaceFileEditorProps {
  controller: WorkspaceFileBrowserController;
  theme: ThemeMode;
  contentAriaLabel: string;
  className?: string;
}

function WorkspaceFileEditor({
  controller,
  theme,
  contentAriaLabel,
  className,
}: WorkspaceFileEditorProps) {
  return (
    <Suspense
      fallback={
        <div
          className={cn(
            "flex min-h-[18rem] min-w-0 items-center justify-center bg-background text-xs text-muted-foreground",
            className
          )}
          data-testid="workspace-file-editor"
          role="region"
          aria-label="File editor"
        >
          Loading editor...
        </div>
      }
    >
      <LazyWorkspaceFileEditor
        controller={controller}
        theme={theme}
        contentAriaLabel={contentAriaLabel}
        className={className}
      />
    </Suspense>
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
  onCreateEntry,
  onMoveEntry,
  onDeleteEntry,
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
    onCreateEntry,
    onMoveEntry,
    onDeleteEntry,
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
