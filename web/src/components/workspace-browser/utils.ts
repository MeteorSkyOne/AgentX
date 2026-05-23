import type { WorkspaceGitChange, WorkspaceGitScope, WorkspaceTreeEntry } from "@/api/types";
import type { WorkspaceFilePosition } from "./types";

export function normalizeWorkspaceFilePosition(
  position?: WorkspaceFilePosition
): WorkspaceFilePosition | undefined {
  if (!position || position.lineNumber < 1) return undefined;
  return {
    lineNumber: position.lineNumber,
    column: position.column && position.column > 0 ? position.column : 1,
  };
}

export function downloadWorkspaceFileBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename || "download";
  link.style.display = "none";
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function workspaceFileDownloadName(path: string): string {
  return path.trim().split(/[\\/]/).pop() || "download";
}

export function normalizeWorkspaceTreePath(path: string): string {
  const normalized = path.trim().replaceAll("\\", "/").replace(/^\/+|\/+$/g, "");
  return normalized === "." ? "" : normalized;
}

export function normalizeWorkspaceEntryName(name: string): string {
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

export function joinWorkspacePath(parentPath: string, name: string): string {
  const normalizedParent = normalizeWorkspaceTreePath(parentPath);
  return normalizedParent ? `${normalizedParent}/${name}` : name;
}

export function workspacePathIsSameOrInside(path: string, parentPath: string): boolean {
  const normalizedPath = normalizeWorkspaceTreePath(path);
  const normalizedParent = normalizeWorkspaceTreePath(parentPath);
  if (!normalizedPath || !normalizedParent) return normalizedPath === normalizedParent;
  return normalizedPath === normalizedParent || normalizedPath.startsWith(`${normalizedParent}/`);
}

export function replaceMovedWorkspacePath(path: string, sourcePath: string, targetPath: string): string {
  const normalizedPath = normalizeWorkspaceTreePath(path);
  const normalizedSource = normalizeWorkspaceTreePath(sourcePath);
  const normalizedTarget = normalizeWorkspaceTreePath(targetPath);
  if (!workspacePathIsSameOrInside(normalizedPath, normalizedSource)) return path;
  return `${normalizedTarget}${normalizedPath.slice(normalizedSource.length)}`;
}

export function parentWorkspaceDirectoryPath(path: string): string {
  const normalized = normalizeWorkspaceTreePath(path);
  const slashIndex = normalized.lastIndexOf("/");
  return slashIndex === -1 ? "" : normalized.slice(0, slashIndex);
}

export function workspaceTreeDirectoryLoaded(tree: WorkspaceTreeEntry | undefined, path: string): boolean {
  const entry = findWorkspaceTreeEntry(tree, path);
  return entry?.type === "directory" && Boolean(entry.children_loaded);
}

export function findWorkspaceTreeEntry(
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

export function uniqueWorkspaceChildName(
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

export function gitChangeCode(change: WorkspaceGitChange): string {
  switch (change.status) {
    case "added":
      return "A";
    case "deleted":
      return "D";
    case "renamed":
      return "R";
    case "copied":
      return "C";
    case "untracked":
      return "?";
    case "typechange":
      return "T";
    default:
      return "M";
  }
}

export function gitChangeStatusLabel(status: WorkspaceGitChange["status"]): string {
  switch (status) {
    case "added":
      return "Added";
    case "deleted":
      return "Deleted";
    case "renamed":
      return "Renamed";
    case "copied":
      return "Copied";
    case "untracked":
      return "Untracked";
    case "typechange":
      return "Type changed";
    default:
      return "Modified";
  }
}

export function gitChangeTone(status: WorkspaceGitChange["status"]): string {
  switch (status) {
    case "added":
    case "untracked":
      return "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
    case "deleted":
      return "border-destructive/40 bg-destructive/10 text-destructive";
    case "renamed":
    case "copied":
      return "border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300";
    default:
      return "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300";
  }
}

export function workspaceEntryNameValidationError(name: string): string | null {
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

export function mergeWorkspaceTreeEntry(
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

export function gitScopeLabel(scope: WorkspaceGitScope): string {
  if (scope === "branch") return "Branch";
  if (scope === "commit") return "Commit";
  return "Working tree";
}

export function shortCommitSHA(sha: string): string {
  return sha.slice(0, 12);
}

export function formatGitCommitDate(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(timestamp));
}
