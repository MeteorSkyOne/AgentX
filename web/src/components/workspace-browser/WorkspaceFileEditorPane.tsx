import { lazy, Suspense, useState } from "react";
import type { ReactNode } from "react";
import {
  ChevronDown,
  ChevronUp,
  Code2,
  Columns2,
  Database,
  Download,
  Eye,
  FileDiff,
  FileText,
  FolderOpen,
  RefreshCw,
  Save,
  Trash2,
} from "lucide-react";
import type { WorkspaceGitScope } from "@/api/types";
import type { ThemeMode } from "@/theme";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { isMarkdownFilePath, isPdfFilePath } from "../workspaceFileLanguages";
import type { WorkspaceFileBrowserController, WorkspaceFileViewMode } from "./types";
import { WorkspaceFileTabBar } from "./WorkspaceFileTabs";

const LazyWorkspaceFileEditor = lazy(() =>
  import("../WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceFileEditor }))
);

const LazyWorkspaceGitDiffViewer = lazy(() =>
  import("../WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceGitDiffViewer }))
);

export function WorkspaceFileEditorPane({
  controller,
  theme,
  title = "Project file",
  contentAriaLabel = "Project file editor",
  className,
  editorClassName,
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
  editorClassName?: string;
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
        <WorkspaceFileTabBar controller={controller} />
        <WorkspaceFileEditor
          controller={controller}
          theme={theme}
          contentAriaLabel={contentAriaLabel}
          className={cn("min-h-0 flex-1", editorClassName)}
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

export function WorkspaceGitDiffPane({
  controller,
  theme,
  title = "Git diff",
  contentAriaLabel = "Git diff preview",
  className,
  viewerClassName,
  toolbarEnd,
}: {
  controller: WorkspaceFileBrowserController;
  theme: ThemeMode;
  title?: string;
  contentAriaLabel?: string;
  className?: string;
  viewerClassName?: string;
  toolbarEnd?: ReactNode;
}) {
  const diff = controller.gitDiff;
  const path = diff?.path || controller.gitSelectedPath || title;
  const [diffNavigationContainer, setDiffNavigationContainer] = useState<HTMLDivElement | null>(null);

  return (
    <div className={cn("flex h-full min-h-0 min-w-0 flex-col bg-background", className)} data-testid="project-git-diff-pane">
      <div className="flex shrink-0 flex-col gap-2 border-b border-border p-3">
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <FileDiff className="h-3.5 w-3.5 shrink-0" />
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-sm font-semibold text-foreground">{path}</h1>
            <p className="truncate">
              {gitScopeLabel(controller.gitScope)}
              {diff?.target || diff?.base
                ? ` - ${diff.target || diff.base} vs ${diff.compare || diff.branch || "HEAD"}`
                : diff?.branch
                  ? ` - ${diff.branch}`
                  : ""}
            </p>
          </div>
          {controller.workspaceStatus && (
            <span className="shrink-0" aria-live="polite">
              {controller.workspaceStatus}
            </span>
          )}
          {diff ? (
            <div
              ref={setDiffNavigationContainer}
              className="flex shrink-0 items-center"
              data-testid="workspace-git-diff-navigation-host"
            />
          ) : null}
          {toolbarEnd}
        </div>
      </div>
      <div className={cn("relative min-h-0 flex-1", viewerClassName)}>
        {controller.gitDiffLoading && (
          <div className="absolute right-3 top-3 z-10 rounded border border-border bg-background/95 px-2 py-1 text-xs text-muted-foreground shadow-sm">
            Loading...
          </div>
        )}
        {controller.gitDiffError && !controller.gitDiffLoading ? (
          <GitDiffMessage
            title="Diff load failed"
            message={controller.gitDiffError}
            onRetry={controller.gitSelectedPath ? () => void controller.loadGitDiff(controller.gitSelectedPath) : undefined}
          />
        ) : diff ? (
          <Suspense
            fallback={
              <div className="flex h-full min-h-[18rem] items-center justify-center text-xs text-muted-foreground">
                Loading diff editor...
              </div>
            }
          >
            <LazyWorkspaceGitDiffViewer
              diff={diff}
              theme={theme}
              contentAriaLabel={contentAriaLabel}
              className="h-full"
              navigationContainer={diffNavigationContainer}
            />
          </Suspense>
        ) : (
          <GitDiffMessage
            title="No change selected"
            message="Select a changed file to preview its diff."
          />
        )}
      </div>
    </div>
  );
}

function GitDiffMessage({
  title,
  message,
  onRetry,
}: {
  title: string;
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex h-full min-h-[18rem] items-center justify-center bg-background p-6" role="status">
      <div className="max-w-md rounded-md border border-border bg-background p-4 text-center shadow-sm">
        <FileDiff className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
        <h2 className="text-sm font-semibold text-foreground">{title}</h2>
        <p className="mt-2 text-xs text-muted-foreground">{message}</p>
        {onRetry && (
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
        )}
      </div>
    </div>
  );
}

function gitScopeLabel(scope: WorkspaceGitScope): string {
  if (scope === "branch") return "Branch";
  if (scope === "commit") return "Commit";
  return "Working tree";
}

function shortCommitSHA(sha: string): string {
  return sha.slice(0, 12);
}

function formatGitCommitDate(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(timestamp));
}

export function WorkspaceFileToolbar({
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
  const pdfFileSelected = isPdfFilePath(controller.trimmedPath);

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
        size="icon-sm"
        variant="outline"
        onClick={() => void controller.downloadFile()}
        disabled={controller.fileDownloading || !controller.canFetchFileBlob || !controller.trimmedPath}
        title="Download file"
        aria-label="Download file"
      >
        <Download className="h-3.5 w-3.5" />
      </Button>
      <Button
        size="sm"
        className="gap-1.5"
        onClick={() => void controller.saveFile()}
        disabled={controller.fileSaving || pdfFileSelected || !controller.canUseWorkspace || !controller.trimmedPath}
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

export function WorkspaceFileEditor({
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

export function WorkspaceFileDeleteDialog({
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
