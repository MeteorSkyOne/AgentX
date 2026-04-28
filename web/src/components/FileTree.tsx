import { useEffect, useMemo, useState } from "react";
import type { DragEvent, MouseEvent, ReactNode } from "react";
import {
  ChevronDown,
  ChevronRight,
  FilePlus2,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  Pencil,
  Trash2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { ScrollArea } from "@/components/ui/scroll-area";

export interface FileTreeEntry {
  name: string;
  path: string;
  type: "directory" | "file";
  has_children?: boolean;
  children_loaded?: boolean;
  children?: FileTreeEntry[];
}

export type FileTreeRow = FileTreeEntry & {
  depth: number;
};

interface FileTreeProps {
  tree?: FileTreeEntry;
  selectedPath?: string;
  loading?: boolean;
  error?: string | null;
  className?: string;
  ariaLabel?: string;
  emptyLabel?: string;
  resetKey?: string | number;
  directoryLoadingPaths?: ReadonlySet<string>;
  directoryErrors?: Record<string, string | null | undefined>;
  canManageEntries?: boolean;
  onSelectFile: (path: string, entry: FileTreeEntry) => void;
  onLoadDirectory?: (path: string, entry: FileTreeEntry) => void;
  onCreateEntry?: (parentPath: string, type: FileTreeEntry["type"]) => void;
  onRenameEntry?: (entry: FileTreeEntry) => void;
  onDeleteEntry?: (entry: FileTreeEntry) => void;
  onMoveEntry?: (entry: FileTreeEntry, targetDirectoryPath: string) => void;
}

export function defaultOpenDirectoryPaths(tree?: FileTreeEntry): string[] {
  void tree;
  return [];
}

export function visibleFileTreeRows(tree: FileTreeEntry, openPaths: ReadonlySet<string>): FileTreeRow[] {
  const rows: FileTreeRow[] = [];

  function visit(entry: FileTreeEntry, depth: number) {
    const isWorkspaceRoot = entry.path === "" && entry.name === "";
    if (!isWorkspaceRoot) {
      rows.push({ ...entry, depth });
    }

    if (entry.type !== "directory") return;
    if (!isWorkspaceRoot && !openPaths.has(entry.path)) return;

    const childDepth = isWorkspaceRoot ? depth : depth + 1;
    for (const child of entry.children ?? []) {
      visit(child, childDepth);
    }
  }

  visit(tree, 0);
  return rows;
}

export function fileTreeHasRows(tree?: FileTreeEntry): boolean {
  if (!tree) return false;
  return visibleFileTreeRows(tree, new Set(defaultOpenDirectoryPaths(tree))).length > 0;
}

export function FileTree({
  tree,
  selectedPath,
  loading = false,
  error,
  className,
  ariaLabel = "Files",
  emptyLabel = "empty",
  resetKey,
  directoryLoadingPaths,
  directoryErrors,
  canManageEntries = true,
  onSelectFile,
  onLoadDirectory,
  onCreateEntry,
  onRenameEntry,
  onDeleteEntry,
  onMoveEntry,
}: FileTreeProps) {
  const [openPaths, setOpenPaths] = useState<Set<string>>(() => new Set(defaultOpenDirectoryPaths(tree)));
  const [contextTargetPath, setContextTargetPath] = useState<string | null>(null);
  const [draggedEntryPath, setDraggedEntryPath] = useState<string | null>(null);
  const [dragOverDirectoryPath, setDragOverDirectoryPath] = useState<string | null>(null);

  useEffect(() => {
    setOpenPaths(new Set(defaultOpenDirectoryPaths(tree)));
  }, [resetKey, tree?.path]);

  const hasRows = useMemo(() => fileTreeHasRows(tree), [tree]);
  const contextTarget = useMemo(
    () => findFileTreeEntry(tree, contextTargetPath),
    [contextTargetPath, tree]
  );
  const entryActionsEnabled = canManageEntries && Boolean(tree);
  const canDragEntries = entryActionsEnabled && Boolean(onMoveEntry);

  function setDirectoryOpen(entry: FileTreeEntry, open: boolean) {
    setOpenPaths((current) => {
      const next = new Set(current);
      if (open) {
        next.add(entry.path);
      } else {
        next.delete(entry.path);
      }
      return next;
    });
    if (open && entry.type === "directory" && directoryCanExpand(entry) && !directoryIsLoaded(entry)) {
      onLoadDirectory?.(entry.path, entry);
    }
  }

  const content = (
    <div
      className={cn("overflow-hidden rounded-md border border-border bg-background/50", className)}
      onContextMenu={tree ? handleTreeContextMenu : undefined}
      onDragOver={tree ? (event) => handleDirectoryDragOver(event, "") : undefined}
      onDragLeave={tree ? handleRootDragLeave : undefined}
      onDrop={tree ? (event) => handleDirectoryDrop(event, "") : undefined}
    >
      {error ? (
        <p className="px-3 py-2 text-xs text-destructive">{error}</p>
      ) : loading && !tree ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">Loading...</p>
      ) : !tree ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
      ) : (
        <ScrollArea className="h-full">
          <div
            className={cn(
              "min-h-full p-2",
              dragOverDirectoryPath === "" && "bg-accent/30"
            )}
            role="tree"
            aria-label={ariaLabel}
          >
            {hasRows ? renderEntries(tree, 0) : (
              <p className="px-1 py-1 text-xs text-muted-foreground">{emptyLabel}</p>
            )}
          </div>
        </ScrollArea>
      )}
    </div>
  );

  if (!tree) return content;

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{content}</ContextMenuTrigger>
      {renderContextMenu()}
    </ContextMenu>
  );

  function handleTreeContextMenu(event: MouseEvent<HTMLElement>) {
    const target = event.target instanceof HTMLElement
      ? event.target.closest<HTMLElement>("[data-file-tree-path]")
      : null;
    setContextTargetPath(target?.dataset.fileTreePath ?? null);
  }

  function renderContextMenu() {
    const target = contextTarget;
    const parentPath = target?.type === "directory"
      ? target.path
      : parentFileTreeDirectoryPath(target?.path ?? "");
    const canCreate = entryActionsEnabled && Boolean(onCreateEntry);
    const canRename = entryActionsEnabled && Boolean(target && onRenameEntry);
    const canDelete = entryActionsEnabled && Boolean(target && onDeleteEntry);

    return (
      <ContextMenuContent>
        <ContextMenuItem disabled={!canCreate} onSelect={() => onCreateEntry?.(parentPath, "file")}>
          <FilePlus2 className="h-3.5 w-3.5" />
          New file
        </ContextMenuItem>
        <ContextMenuItem disabled={!canCreate} onSelect={() => onCreateEntry?.(parentPath, "directory")}>
          <FolderPlus className="h-3.5 w-3.5" />
          New folder
        </ContextMenuItem>
        {target && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem disabled={!canRename} onSelect={() => onRenameEntry?.(target)}>
              <Pencil className="h-3.5 w-3.5" />
              Rename
            </ContextMenuItem>
            <ContextMenuItem
              disabled={!canDelete}
              variant="destructive"
              onSelect={() => onDeleteEntry?.(target)}
            >
              <Trash2 className="h-3.5 w-3.5" />
              Delete
            </ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    );
  }

  function renderEntries(entry: FileTreeEntry, depth: number): ReactNode {
    const isWorkspaceRoot = entry.path === "" && entry.name === "";
    if (isWorkspaceRoot) {
      return (entry.children ?? []).map((child) => renderEntries(child, depth));
    }
    if (entry.type === "directory") {
      return renderDirectory(entry, depth);
    }
    return renderFile(entry, depth);
  }

  function renderDirectory(entry: FileTreeEntry, depth: number) {
    const children = entry.children ?? [];
    const canExpand = directoryCanExpand(entry);
    const loaded = directoryIsLoaded(entry);
    const loading = directoryLoadingPaths?.has(entry.path) ?? false;
    const directoryError = directoryErrors?.[entry.path] ?? null;
    const open = openPaths.has(entry.path);

    return (
      <Collapsible
        key={entry.path}
        open={open}
        onOpenChange={(nextOpen) => setDirectoryOpen(entry, nextOpen)}
      >
        <CollapsibleTrigger asChild>
          <button
            className={cn(
              "flex h-7 w-full items-center gap-1.5 rounded px-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground disabled:hover:bg-transparent disabled:hover:text-muted-foreground",
              dragOverDirectoryPath === entry.path && "bg-accent text-foreground ring-1 ring-ring/30"
            )}
            style={{ paddingLeft: `${depth * 14 + 6}px` }}
            type="button"
            role="treeitem"
            aria-expanded={canExpand || loading ? open : undefined}
            aria-busy={loading || undefined}
            data-file-tree-path={entry.path}
            draggable={canDragEntries}
            onDragStart={(event) => handleDragStart(event, entry)}
            onDragEnd={handleDragEnd}
            onDragOver={(event) => handleDirectoryDragOver(event, entry.path)}
            onDragLeave={(event) => handleDirectoryDragLeave(event, entry.path)}
            onDrop={(event) => handleDirectoryDrop(event, entry.path)}
          >
            {canExpand || loading ? (
              open ? (
                <ChevronDown className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5 shrink-0" />
              )
            ) : (
              <span className="h-3.5 w-3.5 shrink-0" />
            )}
            {open ? (
              <FolderOpen className="h-3.5 w-3.5 shrink-0 text-yellow-500" />
            ) : (
              <Folder className="h-3.5 w-3.5 shrink-0 text-yellow-500" />
            )}
            <span className="truncate">{entry.name}</span>
          </button>
        </CollapsibleTrigger>
        {(canExpand || loading || directoryError) && (
          <CollapsibleContent role="group">
            {open && loading && !loaded && children.length === 0 && (
              <p
                className="h-7 truncate px-1.5 text-xs text-muted-foreground"
                style={{ paddingLeft: `${(depth + 1) * 14 + 27}px` }}
              >
                Loading...
              </p>
            )}
            {open && directoryError && (
              <p
                className="h-7 truncate px-1.5 text-xs text-destructive"
                style={{ paddingLeft: `${(depth + 1) * 14 + 27}px` }}
              >
                {directoryError}
              </p>
            )}
            {children.map((child) => renderEntries(child, depth + 1))}
          </CollapsibleContent>
        )}
      </Collapsible>
    );
  }

  function renderFile(entry: FileTreeEntry, depth: number) {
    const selected = entry.path === selectedPath;

    return (
      <button
        key={entry.path}
        className={cn(
          "flex h-7 w-full items-center gap-1.5 rounded px-1.5 text-left text-xs transition-colors",
          selected
            ? "bg-accent text-foreground"
            : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
        )}
        style={{ paddingLeft: `${depth * 14 + 27}px` }}
        type="button"
        role="treeitem"
        aria-selected={selected}
        data-file-tree-path={entry.path}
        draggable={canDragEntries}
        onDragStart={(event) => handleDragStart(event, entry)}
        onDragEnd={handleDragEnd}
        onDragOver={handleFileDragOver}
        onDrop={handleFileDrop}
        onClick={() => onSelectFile(entry.path, entry)}
      >
        <FileText className="h-3.5 w-3.5 shrink-0 text-blue-400" />
        <span className="truncate">{entry.name}</span>
      </button>
    );
  }

  function handleDragStart(event: DragEvent<HTMLElement>, entry: FileTreeEntry) {
    if (!canDragEntries) return;
    setDraggedEntryPath(entry.path);
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("application/x-agentx-workspace-entry", entry.path);
    event.dataTransfer.setData("text/plain", entry.path);
  }

  function handleDragEnd() {
    setDraggedEntryPath(null);
    setDragOverDirectoryPath(null);
  }

  function handleDirectoryDragOver(event: DragEvent<HTMLElement>, targetDirectoryPath: string) {
    const source = draggedFileTreeEntry(event);
    if (!source) return;
    event.stopPropagation();
    if (!canDropFileTreeEntry(source, targetDirectoryPath)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDragOverDirectoryPath(normalizeFileTreePath(targetDirectoryPath));
  }

  function handleDirectoryDragLeave(event: DragEvent<HTMLElement>, targetDirectoryPath: string) {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    setDragOverDirectoryPath((current) =>
      current === normalizeFileTreePath(targetDirectoryPath) ? null : current
    );
  }

  function handleRootDragLeave(event: DragEvent<HTMLElement>) {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    setDragOverDirectoryPath(null);
  }

  function handleDirectoryDrop(event: DragEvent<HTMLElement>, targetDirectoryPath: string) {
    const source = draggedFileTreeEntry(event);
    if (!source) return;
    event.stopPropagation();
    if (!canDropFileTreeEntry(source, targetDirectoryPath)) return;
    event.preventDefault();
    setDraggedEntryPath(null);
    setDragOverDirectoryPath(null);
    onMoveEntry?.(source, normalizeFileTreePath(targetDirectoryPath));
  }

  function draggedFileTreeEntry(event: DragEvent<HTMLElement>): FileTreeEntry | undefined {
    const sourcePath = draggedEntryPath
      || event.dataTransfer.getData("application/x-agentx-workspace-entry")
      || event.dataTransfer.getData("text/plain");
    return findFileTreeEntry(tree, sourcePath);
  }

  function handleFileDragOver(event: DragEvent<HTMLElement>) {
    if (!draggedFileTreeEntry(event)) return;
    event.stopPropagation();
  }

  function handleFileDrop(event: DragEvent<HTMLElement>) {
    if (!draggedFileTreeEntry(event)) return;
    event.preventDefault();
    event.stopPropagation();
    setDraggedEntryPath(null);
    setDragOverDirectoryPath(null);
  }

  function canDropFileTreeEntry(entry: FileTreeEntry, targetDirectoryPath: string): boolean {
    if (!canDragEntries) return false;
    const targetPath = normalizeFileTreePath(targetDirectoryPath);
    if (parentFileTreeDirectoryPath(entry.path) === targetPath) return false;
    if (entry.type === "directory" && fileTreePathIsSameOrInside(targetPath, entry.path)) {
      return false;
    }
    return true;
  }
}

function directoryCanExpand(entry: FileTreeEntry): boolean {
  return entry.type === "directory" && (entry.has_children || Boolean(entry.children?.length));
}

function directoryIsLoaded(entry: FileTreeEntry): boolean {
  return entry.type === "directory" && (entry.children_loaded || Boolean(entry.children?.length));
}

function findFileTreeEntry(
  entry: FileTreeEntry | undefined,
  path: string | null
): FileTreeEntry | undefined {
  if (!entry || path === null) return undefined;
  if (entry.path === path) return entry;
  for (const child of entry.children ?? []) {
    if (child.path === path || path.startsWith(`${child.path}/`)) {
      const match = findFileTreeEntry(child, path);
      if (match) return match;
    }
  }
  return undefined;
}

function normalizeFileTreePath(path: string): string {
  const normalized = path.trim().replaceAll("\\", "/").replace(/^\/+|\/+$/g, "");
  return normalized === "." ? "" : normalized;
}

function parentFileTreeDirectoryPath(path: string): string {
  const normalized = normalizeFileTreePath(path);
  const slashIndex = normalized.lastIndexOf("/");
  return slashIndex === -1 ? "" : normalized.slice(0, slashIndex);
}

function fileTreePathIsSameOrInside(path: string, parentPath: string): boolean {
  const normalizedPath = normalizeFileTreePath(path);
  const normalizedParent = normalizeFileTreePath(parentPath);
  if (!normalizedPath || !normalizedParent) return normalizedPath === normalizedParent;
  return normalizedPath === normalizedParent || normalizedPath.startsWith(`${normalizedParent}/`);
}
