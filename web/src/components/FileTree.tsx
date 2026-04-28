import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen } from "lucide-react";
import { cn } from "@/lib/utils";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
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
  onSelectFile: (path: string, entry: FileTreeEntry) => void;
  onLoadDirectory?: (path: string, entry: FileTreeEntry) => void;
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
  onSelectFile,
  onLoadDirectory
}: FileTreeProps) {
  const [openPaths, setOpenPaths] = useState<Set<string>>(() => new Set(defaultOpenDirectoryPaths(tree)));

  useEffect(() => {
    setOpenPaths(new Set(defaultOpenDirectoryPaths(tree)));
  }, [resetKey, tree?.path]);

  const hasRows = useMemo(() => fileTreeHasRows(tree), [tree]);

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

  return (
    <div className={cn("overflow-hidden rounded-md border border-border bg-background/50", className)}>
      {error ? (
        <p className="px-3 py-2 text-xs text-destructive">{error}</p>
      ) : loading && !tree ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">Loading...</p>
      ) : !tree || !hasRows ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
      ) : (
        <ScrollArea className="h-full">
          <div className="p-2" role="tree" aria-label={ariaLabel}>
            {renderEntries(tree, 0)}
          </div>
        </ScrollArea>
      )}
    </div>
  );

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
        <CollapsibleTrigger asChild disabled={!canExpand && !loading}>
          <button
            className="flex h-7 w-full items-center gap-1.5 rounded px-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
            style={{ paddingLeft: `${depth * 14 + 6}px` }}
            type="button"
            role="treeitem"
            aria-expanded={canExpand || loading ? open : undefined}
            aria-busy={loading || undefined}
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
        onClick={() => onSelectFile(entry.path, entry)}
      >
        <FileText className="h-3.5 w-3.5 shrink-0 text-blue-400" />
        <span className="truncate">{entry.name}</span>
      </button>
    );
  }
}

function directoryCanExpand(entry: FileTreeEntry): boolean {
  return entry.type === "directory" && (entry.has_children || Boolean(entry.children?.length));
}

function directoryIsLoaded(entry: FileTreeEntry): boolean {
  return entry.type === "directory" && (entry.children_loaded || Boolean(entry.children?.length));
}
