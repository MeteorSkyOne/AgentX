import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import {
  CaseSensitive,
  FileSearch,
  FileText,
  FolderOpen,
  GitBranch,
  GitCompare,
  History,
  Regex,
  RefreshCw,
  Search,
  TextSearch,
  WholeWord,
  X,
} from "lucide-react";
import type {
  WorkspaceEntryType,
  WorkspaceGitChange,
  WorkspaceGitCommit,
  WorkspaceGitScope,
  WorkspaceSearchMode,
  WorkspaceSearchResult,
  WorkspaceTreeEntry,
} from "@/api/types";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { FileTree } from "../FileTree";
import type { WorkspaceFileBrowserController } from "./types";
import {
  formatGitCommitDate,
  gitChangeCode,
  gitChangeStatusLabel,
  gitChangeTone,
  shortCommitSHA,
  uniqueWorkspaceChildName,
  workspaceEntryNameValidationError,
} from "./utils";

const workspaceSearchDebounceMs = 400;

export function WorkspaceFileTreePane({
  controller,
  title = "Project files",
  ariaLabel = "Project files",
  className,
  onFileSelected,
  onChangeSelected,
  toolbarEnd,
}: {
  controller: WorkspaceFileBrowserController;
  title?: string;
  ariaLabel?: string;
  className?: string;
  onFileSelected?: () => void;
  onChangeSelected?: () => void;
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
      <div className="flex min-h-0 flex-1 flex-col gap-3 p-3">
        {controller.gitEnabled && (
          <WorkspacePaneViewSwitch controller={controller} />
        )}
        {controller.workspacePaneView === "files" && controller.canSearchWorkspace && (
          <WorkspaceSearchControls controller={controller} />
        )}
        <div className="min-h-0 flex-1">
          {controller.gitEnabled && controller.workspacePaneView === "changes" ? (
            <WorkspaceGitChangesList
              controller={controller}
              className="h-full"
              onChangeSelected={onChangeSelected}
            />
          ) : controller.canSearchWorkspace && controller.searchQuery.trim() ? (
            <WorkspaceSearchResults
              controller={controller}
              className="h-full"
              onFileSelected={onFileSelected}
            />
          ) : (
            <WorkspaceFileTree
              controller={controller}
              ariaLabel={ariaLabel}
              className="h-full"
              onFileSelected={onFileSelected}
            />
          )}
        </div>
      </div>
    </div>
  );
}

function WorkspaceSearchControls({
  controller,
}: {
  controller: WorkspaceFileBrowserController;
}) {
  const query = controller.searchQuery.trim();

  useEffect(() => {
    if (!controller.canSearchWorkspace) return;
    if (!query) {
      controller.loadSearch({ quiet: true });
      return;
    }
    const timeout = window.setTimeout(() => {
      void controller.loadSearch({ quiet: true });
    }, workspaceSearchDebounceMs);
    return () => window.clearTimeout(timeout);
  }, [
    controller.canSearchWorkspace,
    controller.searchCaseSensitive,
    controller.searchMode,
    controller.searchRegex,
    controller.searchWholeWord,
    query,
  ]);

  return (
    <div className="flex shrink-0 flex-col gap-2 rounded-md border border-border bg-background/50 p-2">
      <div className="flex min-w-0 items-center gap-1.5">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={controller.searchQuery}
            onChange={(event) => controller.setSearchQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                void controller.loadSearch();
              } else if (event.key === "Escape") {
                controller.clearSearch();
              }
            }}
            placeholder={controller.searchMode === "content" ? "Search content" : "Search files"}
            aria-label="Search project files"
            className="h-8 pl-7 pr-8 text-xs"
          />
          {controller.searchQuery ? (
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              className="absolute right-0.5 top-1/2 h-7 w-7 -translate-y-1/2"
              title="Clear search"
              aria-label="Clear search"
              onClick={controller.clearSearch}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          ) : null}
        </div>
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          title="Run search"
          aria-label="Run search"
          disabled={!query || controller.searchLoading}
          onClick={() => void controller.loadSearch()}
        >
          <RefreshCw className={cn("h-3.5 w-3.5", controller.searchLoading && "animate-spin")} />
        </Button>
      </div>
      <div className="flex min-w-0 items-center justify-between gap-2">
        <div
          className="grid grid-cols-2 rounded-md border border-border bg-background p-0.5"
          role="group"
          aria-label="Search mode"
        >
          <WorkspaceSearchModeButton
            selected={controller.searchMode === "files"}
            label="Search filenames"
            onClick={() => controller.setSearchMode("files")}
          >
            <FileSearch className="h-3.5 w-3.5" />
            Files
          </WorkspaceSearchModeButton>
          <WorkspaceSearchModeButton
            selected={controller.searchMode === "content"}
            label="Search file content"
            onClick={() => controller.setSearchMode("content")}
          >
            <TextSearch className="h-3.5 w-3.5" />
            Text
          </WorkspaceSearchModeButton>
        </div>
        <div className="flex shrink-0 items-center gap-1" role="group" aria-label="Search options">
          <WorkspaceSearchToggle
            selected={controller.searchCaseSensitive}
            label="Match case"
            onClick={() => controller.setSearchCaseSensitive(!controller.searchCaseSensitive)}
          >
            <CaseSensitive className="h-3.5 w-3.5" />
          </WorkspaceSearchToggle>
          <WorkspaceSearchToggle
            selected={controller.searchWholeWord}
            label="Match whole word"
            onClick={() => controller.setSearchWholeWord(!controller.searchWholeWord)}
          >
            <WholeWord className="h-3.5 w-3.5" />
          </WorkspaceSearchToggle>
          <WorkspaceSearchToggle
            selected={controller.searchRegex}
            label="Use regular expression"
            onClick={() => controller.setSearchRegex(!controller.searchRegex)}
          >
            <Regex className="h-3.5 w-3.5" />
          </WorkspaceSearchToggle>
        </div>
      </div>
    </div>
  );
}

function WorkspaceSearchModeButton({
  selected,
  label,
  onClick,
  children,
}: {
  selected: boolean;
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Button
      type="button"
      size="sm"
      variant={selected ? "secondary" : "ghost"}
      className="h-7 justify-center gap-1.5 px-2 text-xs"
      aria-label={label}
      aria-pressed={selected}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function WorkspaceSearchToggle({
  selected,
  label,
  onClick,
  children,
}: {
  selected: boolean;
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Button
      type="button"
      size="icon-sm"
      variant={selected ? "secondary" : "ghost"}
      className="h-7 w-7"
      title={label}
      aria-label={label}
      aria-pressed={selected}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function WorkspaceSearchResults({
  controller,
  className,
  onFileSelected,
}: {
  controller: WorkspaceFileBrowserController;
  className?: string;
  onFileSelected?: () => void;
}) {
  const results = controller.searchResults;
  const query = controller.searchQuery.trim();

  return (
    <div className={cn("flex min-h-0 flex-col overflow-hidden rounded-md border border-border bg-background/50", className)}>
      <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border px-2 text-[11px] text-muted-foreground">
        <Search className="h-3 w-3 shrink-0" />
        <span className="min-w-0 flex-1 truncate">
          {controller.searchLoading
            ? "Searching..."
            : controller.searchError
              ? "Search failed"
              : `${results.length} result${results.length === 1 ? "" : "s"}${controller.searchTruncated ? " (limited)" : ""}`}
        </span>
        {controller.searchEngine && (
          <span className="shrink-0 uppercase">{controller.searchEngine}</span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-2" role="list" aria-label="Search results">
        {controller.searchError ? (
          <p className="px-1 py-1 text-xs text-destructive">{controller.searchError}</p>
        ) : controller.searchLoading && results.length === 0 ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">Searching...</p>
        ) : !query ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">Enter a search query.</p>
        ) : results.length === 0 ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">No results.</p>
        ) : (
          results.map((result, index) => (
            <WorkspaceSearchResultRow
              key={workspaceSearchResultKey(result, index)}
              result={result}
              selected={controller.filePath === result.path}
              mode={controller.searchMode}
              onSelect={() => {
                void controller.loadFile(result.path, {
                  preview: true,
                  ...(result.line_number ? {
                    position: {
                      lineNumber: result.line_number,
                      column: result.column ?? 1,
                    },
                  } : undefined),
                });
                onFileSelected?.();
              }}
            />
          ))
        )}
      </div>
    </div>
  );
}

function WorkspaceSearchResultRow({
  result,
  selected,
  mode,
  onSelect,
}: {
  result: WorkspaceSearchResult;
  selected: boolean;
  mode: WorkspaceSearchMode;
  onSelect: () => void;
}) {
  const isContent = mode === "content" && result.line_number;
  return (
    <div role="listitem">
      <button
        type="button"
        className={cn(
          "flex min-h-8 w-full items-start gap-2 rounded px-1.5 py-1 text-left text-xs transition-colors",
          selected
            ? "bg-accent text-foreground"
            : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
        )}
        aria-label={isContent ? `${result.path} line ${result.line_number}` : result.path}
        onClick={onSelect}
      >
        {isContent ? (
          <TextSearch className="mt-0.5 h-3.5 w-3.5 shrink-0 text-blue-400" />
        ) : (
          <FileText className="mt-0.5 h-3.5 w-3.5 shrink-0 text-blue-400" />
        )}
        <span className="min-w-0 flex-1">
          <span className="block truncate">
            {result.path}
            {isContent ? (
              <span className="text-muted-foreground">:{result.line_number}</span>
            ) : null}
          </span>
          {isContent && result.preview ? (
            <span className="block truncate font-mono text-[11px] text-muted-foreground">
              {result.preview}
            </span>
          ) : null}
        </span>
      </button>
    </div>
  );
}

function workspaceSearchResultKey(result: WorkspaceSearchResult, index: number): string {
  return `${result.path}:${result.line_number ?? 0}:${result.column ?? 0}:${index}`;
}

function WorkspacePaneViewSwitch({
  controller,
}: {
  controller: WorkspaceFileBrowserController;
}) {
  return (
    <div
      className="grid grid-cols-2 rounded-md border border-border bg-background p-0.5"
      role="group"
      aria-label="Project files view"
    >
      <WorkspacePaneViewButton
        selected={controller.workspacePaneView === "files"}
        label="Files"
        onClick={() => controller.setWorkspacePaneView("files")}
      >
        <FolderOpen className="h-3.5 w-3.5" />
        Files
      </WorkspacePaneViewButton>
      <WorkspacePaneViewButton
        selected={controller.workspacePaneView === "changes"}
        label="Changes"
        onClick={() => controller.setWorkspacePaneView("changes")}
      >
        <GitCompare className="h-3.5 w-3.5" />
        Changes
      </WorkspacePaneViewButton>
    </div>
  );
}

function WorkspacePaneViewButton({
  selected,
  label,
  onClick,
  children,
}: {
  selected: boolean;
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Button
      type="button"
      size="sm"
      variant={selected ? "secondary" : "ghost"}
      className="h-7 justify-center gap-1.5 px-2 text-xs"
      aria-label={label}
      aria-pressed={selected}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function WorkspaceGitChangesList({
  controller,
  className,
  onChangeSelected,
}: {
  controller: WorkspaceFileBrowserController;
  className?: string;
  onChangeSelected?: () => void;
}) {
  const status =
    controller.gitStatus?.scope === controller.gitScope &&
    (controller.gitScope !== "commit" || controller.gitStatus.commit === controller.gitSelectedCommit)
      ? controller.gitStatus
      : undefined;
  const changes = status?.changes ?? [];
  const branchTargets = status?.targets ?? [];
  const baseBranch = branchTargets.length > 0 ? controller.gitTarget || status?.target || status?.base || "" : "";
  const compareBranch = branchTargets.length > 0 ? controller.gitCompare || status?.compare || status?.branch || "" : "";
  const showCommitHistory = controller.gitScope === "commit";

  return (
    <div className={cn("flex min-h-0 flex-col overflow-hidden rounded-md border border-border bg-background/50", className)}>
      <div className="flex shrink-0 flex-col gap-2 border-b border-border p-2">
        <div className="flex items-center gap-2">
          <Select
            value={controller.gitScope}
            onChange={(event) => controller.setGitScope(event.target.value as WorkspaceGitScope)}
            className="min-w-0 flex-1"
            selectClassName="h-8 text-xs"
            aria-label="Git change scope"
          >
            <option value="working_tree">Working tree</option>
            <option value="branch">Branch</option>
            <option value="commit">Commit</option>
          </Select>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            title="Refresh changes"
            aria-label="Refresh changes"
            disabled={controller.gitStatusLoading || !controller.canUseWorkspace}
            onClick={() => void controller.loadGitStatus()}
          >
            <RefreshCw className={cn("h-3.5 w-3.5", controller.gitStatusLoading && "animate-spin")} />
          </Button>
        </div>
        {controller.gitScope === "branch" && (
          <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-1.5">
            <Select
              value={baseBranch}
              onChange={(event) => controller.setGitTarget(event.target.value)}
              className="min-w-0"
              selectClassName="h-8 text-xs"
              aria-label="Base branch"
              disabled={controller.gitStatusLoading && !status}
            >
              {branchTargets.length > 0 ? (
                branchTargets.map((target) => (
                  <option key={target.name} value={target.name}>
                    {target.name}
                  </option>
                ))
              ) : (
                <option value="">No branch</option>
              )}
            </Select>
            <span className="shrink-0 text-[11px] text-muted-foreground">vs</span>
            <Select
              value={compareBranch}
              onChange={(event) => controller.setGitCompare(event.target.value)}
              className="min-w-0"
              selectClassName="h-8 text-xs"
              aria-label="Compare branch"
              disabled={controller.gitStatusLoading && !status}
            >
              {branchTargets.length > 0 ? (
                branchTargets.map((target) => (
                  <option key={target.name} value={target.name}>
                    {target.name}
                  </option>
                ))
              ) : (
                <option value="">No branch</option>
              )}
            </Select>
          </div>
        )}
        {showCommitHistory && (
          <div className="grid grid-cols-2 gap-1.5">
            <Button
              type="button"
              size="sm"
              variant={controller.gitHistoryMode === "repository" ? "secondary" : "ghost"}
              aria-pressed={controller.gitHistoryMode === "repository"}
              className="h-8 gap-1.5 text-xs"
              onClick={() => controller.setGitHistoryMode("repository")}
            >
              <History className="h-3.5 w-3.5" />
              Repository
            </Button>
            <Button
              type="button"
              size="sm"
              variant={controller.gitHistoryMode === "file" ? "secondary" : "ghost"}
              aria-pressed={controller.gitHistoryMode === "file"}
              className="h-8 gap-1.5 text-xs"
              onClick={() => controller.setGitHistoryMode("file")}
            >
              <FileText className="h-3.5 w-3.5" />
              Current file
            </Button>
          </div>
        )}
      </div>
      {status?.available && (
        <div className="flex shrink-0 items-center gap-1.5 border-b border-border px-2 py-1.5 text-[11px] text-muted-foreground">
          <GitBranch className="h-3 w-3 shrink-0" />
          {controller.gitScope === "commit" && status.commit ? (
            <>
              <span className="truncate">{shortCommitSHA(status.commit)}</span>
              <span className="shrink-0">vs</span>
              <span className="truncate">{status.base ? shortCommitSHA(status.base) : "empty tree"}</span>
            </>
          ) : controller.gitScope === "branch" && (status.target || status.base) ? (
            <>
              <span className="truncate">{status.target || status.base}</span>
              <span className="shrink-0">vs</span>
              <span className="truncate">{status.compare || status.branch || "HEAD"}</span>
            </>
          ) : (
            <span className="truncate">{status.branch || "HEAD"}</span>
          )}
        </div>
      )}
      {showCommitHistory && (
        <WorkspaceGitCommitHistory controller={controller} />
      )}
      <div className={cn("min-h-0 overflow-hidden", showCommitHistory ? "flex-[1.2]" : "flex-1")}>
        {controller.gitStatusError ? (
          <p className="px-3 py-2 text-xs text-destructive">{controller.gitStatusError}</p>
        ) : controller.gitStatusLoading && !status ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">Loading...</p>
        ) : showCommitHistory && !controller.gitSelectedCommit ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">Select a commit to view changed files.</p>
        ) : status && !status.available ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">{status.message || "Git changes are unavailable."}</p>
        ) : !status ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">Select Changes to load git status.</p>
        ) : changes.length === 0 ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">No changes.</p>
        ) : (
          <div className="h-full overflow-auto p-2" role="list" aria-label="Git changes">
            {changes.map((change) => (
              <WorkspaceGitChangeRow
                key={`${change.old_path ?? ""}:${change.path}:${change.status}`}
                change={change}
                selected={controller.gitSelectedPath === change.path}
                onSelect={() => {
                  void controller.loadGitDiff(change.path);
                  onChangeSelected?.();
                }}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function WorkspaceGitCommitHistory({
  controller,
}: {
  controller: WorkspaceFileBrowserController;
}) {
  const history = controller.gitHistory;
  const commits = history?.commits ?? [];
  const query = controller.gitHistoryQuery.trim();

  useEffect(() => {
    if (controller.gitScope !== "commit") return;
    const timeout = window.setTimeout(() => {
      void controller.loadGitHistory({ quiet: true });
    }, query ? workspaceSearchDebounceMs : 0);
    return () => window.clearTimeout(timeout);
  }, [controller.gitScope, controller.gitHistoryMode, controller.gitHistoryQuery, controller.trimmedPath]);

  return (
    <div className="flex min-h-0 flex-1 flex-col border-b border-border">
      <div className="flex shrink-0 flex-col gap-1.5 border-b border-border p-2">
        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <History className="h-3 w-3 shrink-0" />
          <span className="min-w-0 flex-1 truncate">
            {controller.gitHistoryMode === "file"
              ? controller.trimmedPath || "Current file"
              : history?.branch || "HEAD"}
          </span>
          {history?.has_more && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-[11px]"
              disabled={controller.gitHistoryLoading}
              onClick={() => void controller.loadGitHistory({
                quiet: true,
                offset: commits.length,
                append: true,
              })}
            >
              More
            </Button>
          )}
        </div>
        <Input
          value={controller.gitHistoryQuery}
          onChange={(event) => controller.setGitHistoryQuery(event.target.value)}
          placeholder="Search commits"
          aria-label="Search commits"
          className="h-8 text-xs"
        />
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-2" role="list" aria-label="Git commit history">
        {controller.gitHistoryError ? (
          <p className="px-1 py-1 text-xs text-destructive">{controller.gitHistoryError}</p>
        ) : controller.gitHistoryLoading && !history ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">Loading...</p>
        ) : history && !history.available ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">{history.message || "Git history is unavailable."}</p>
        ) : history?.message && commits.length === 0 ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">{history.message}</p>
        ) : commits.length === 0 ? (
          <p className="px-1 py-1 text-xs text-muted-foreground">{query ? "No matching commits." : "No commits."}</p>
        ) : (
          commits.map((commit) => (
            <WorkspaceGitCommitRow
              key={commit.sha}
              commit={commit}
              selected={controller.gitSelectedCommit === commit.sha}
              onSelect={() => void controller.selectGitCommit(commit)}
            />
          ))
        )}
      </div>
    </div>
  );
}

function WorkspaceGitCommitRow({
  commit,
  selected,
  onSelect,
}: {
  commit: WorkspaceGitCommit;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <div role="listitem" aria-label={`${commit.short_sha} ${commit.subject}`}>
      <button
        type="button"
        className={cn(
          "flex min-h-10 w-full items-start gap-2 rounded px-1.5 py-1.5 text-left text-xs transition-colors",
          selected
            ? "bg-accent text-foreground"
            : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
        )}
        onClick={onSelect}
      >
        <span className="mt-0.5 shrink-0 font-mono text-[10px] text-muted-foreground">
          {commit.short_sha}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium text-foreground">{commit.subject || "(no subject)"}</span>
          <span className="block truncate text-[11px] text-muted-foreground">
            {commit.author_name}
            {commit.authored_at ? ` - ${formatGitCommitDate(commit.authored_at)}` : ""}
          </span>
        </span>
      </button>
    </div>
  );
}

function WorkspaceGitChangeRow({
  change,
  selected,
  onSelect,
}: {
  change: WorkspaceGitChange;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <div
      role="listitem"
      aria-label={`${gitChangeStatusLabel(change.status)} ${change.path}`}
    >
      <button
        type="button"
        className={cn(
          "flex min-h-8 w-full items-center gap-2 rounded px-1.5 py-1 text-left text-xs transition-colors",
          selected
            ? "bg-accent text-foreground"
            : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
        )}
        onClick={onSelect}
      >
        <span
          className={cn(
            "flex h-5 min-w-5 shrink-0 items-center justify-center rounded border px-1 text-[10px] font-semibold uppercase",
            gitChangeTone(change.status)
          )}
        >
          {gitChangeCode(change)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate">{change.path}</span>
          {change.old_path && (
            <span className="block truncate text-[11px] text-muted-foreground">
              from {change.old_path}
            </span>
          )}
        </span>
      </button>
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
          void controller.loadFile(path, { preview: true });
          onFileSelected?.();
        }}
        onDoubleClickFile={(path) => {
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
