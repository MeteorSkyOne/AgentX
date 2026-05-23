import { useCallback, useEffect, useRef, useState } from "react";
import type {
  WorkspaceEntryType,
  WorkspaceGitCommit,
  WorkspaceGitDiff,
  WorkspaceGitHistory,
  WorkspaceGitHistoryMode,
  WorkspaceGitScope,
  WorkspaceGitStatus,
  WorkspaceSearchMode,
  WorkspaceSearchResponse,
  WorkspaceSearchResult,
  WorkspaceTreeEntry,
} from "@/api/types";
import { isPdfFilePath } from "../workspaceFileLanguages";
import type {
  WorkspaceFileBrowserController,
  WorkspaceFileBrowserProps,
  WorkspaceFileOpenOptions,
  WorkspaceFileTab,
  WorkspaceFileViewMode,
  WorkspacePaneView,
} from "./types";
import {
  downloadWorkspaceFileBlob,
  joinWorkspacePath,
  mergeWorkspaceTreeEntry,
  normalizeWorkspaceEntryName,
  normalizeWorkspaceFilePosition,
  normalizeWorkspaceTreePath,
  parentWorkspaceDirectoryPath,
  replaceMovedWorkspacePath,
  uniqueWorkspaceChildName,
  workspaceFileDownloadName,
  workspacePathIsSameOrInside,
  workspaceTreeDirectoryLoaded,
} from "./utils";

export function useWorkspaceFileBrowser({
  workspaceID,
  workspacePath,
  initialPath = "",
  autoLoadTree = true,
  onLoadTree,
  onSearchWorkspace,
  onReadFile,
  onFetchFileBlob,
  onWriteFile,
  onDeleteFile,
  onCreateEntry,
  onMoveEntry,
  onDeleteEntry,
  onLoadGitStatus,
  onLoadGitHistory,
  onLoadGitDiff,
}: Omit<WorkspaceFileBrowserProps, "theme"> & { autoLoadTree?: boolean }): WorkspaceFileBrowserController {
  const [tabs, setTabs] = useState<WorkspaceFileTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string | null>(null);
  const nextTabIdRef = useRef(0);
  const tabFileRequestRef = useRef<Record<string, number>>({});
  const tabsRef = useRef(tabs);
  tabsRef.current = tabs;
  const activeTabIdRef = useRef(activeTabId);
  activeTabIdRef.current = activeTabId;

  const activeTab = tabs.find((t) => t.id === activeTabId) ?? null;

  const filePath = activeTab?.filePath ?? "";
  const fileBody = activeTab?.fileBody ?? "";
  const fileLoading = activeTab?.fileLoading ?? false;
  const fileLoadError = activeTab?.fileLoadError ?? null;
  const fileOpenPosition = activeTab?.fileOpenPosition;
  const fileOpenRequestID = activeTab?.fileOpenRequestID ?? 0;
  const fileViewMode = activeTab?.fileViewMode ?? "edit";

  const [tree, setTree] = useState<WorkspaceTreeEntry>();
  const [workspaceTreeResetKey, setWorkspaceTreeResetKey] = useState(0);
  const [workspaceTreeLoading, setWorkspaceTreeLoading] = useState(false);
  const [workspaceTreeError, setWorkspaceTreeError] = useState<string | null>(null);
  const [directoryLoadingPaths, setDirectoryLoadingPaths] = useState<Set<string>>(() => new Set());
  const [directoryLoadErrors, setDirectoryLoadErrors] = useState<Record<string, string | null | undefined>>({});
  const [searchQuery, setSearchQuery] = useState("");
  const [searchMode, setSearchMode] = useState<WorkspaceSearchMode>("files");
  const [searchCaseSensitive, setSearchCaseSensitive] = useState(false);
  const [searchRegex, setSearchRegex] = useState(false);
  const [searchWholeWord, setSearchWholeWord] = useState(false);
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [searchResults, setSearchResults] = useState<WorkspaceSearchResult[]>([]);
  const [searchTruncated, setSearchTruncated] = useState(false);
  const [searchEngine, setSearchEngine] = useState<WorkspaceSearchResponse["engine"]>();
  const [fileSaving, setFileSaving] = useState(false);
  const [fileDownloading, setFileDownloading] = useState(false);
  const [fileDeleting, setFileDeleting] = useState(false);
  const [entryActionPending, setEntryActionPending] = useState(false);
  const [workspaceStatus, setWorkspaceStatus] = useState<string | null>(null);
  const [workspacePaneView, setWorkspacePaneViewState] = useState<WorkspacePaneView>("files");
  const [gitScope, setGitScopeState] = useState<WorkspaceGitScope>("working_tree");
  const [gitTarget, setGitTargetState] = useState("");
  const [gitCompare, setGitCompareState] = useState("");
  const [gitHistoryMode, setGitHistoryModeState] = useState<WorkspaceGitHistoryMode>("repository");
  const [gitHistoryQuery, setGitHistoryQueryState] = useState("");
  const [gitHistory, setGitHistory] = useState<WorkspaceGitHistory>();
  const [gitHistoryLoading, setGitHistoryLoading] = useState(false);
  const [gitHistoryError, setGitHistoryError] = useState<string | null>(null);
  const [gitSelectedCommit, setGitSelectedCommit] = useState("");
  const [gitStatus, setGitStatus] = useState<WorkspaceGitStatus>();
  const [gitStatusLoading, setGitStatusLoading] = useState(false);
  const [gitStatusError, setGitStatusError] = useState<string | null>(null);
  const [gitDiff, setGitDiff] = useState<WorkspaceGitDiff>();
  const [gitDiffLoading, setGitDiffLoading] = useState(false);
  const [gitDiffError, setGitDiffError] = useState<string | null>(null);
  const [gitSelectedPath, setGitSelectedPath] = useState("");
  const workspaceTreeRequestRef = useRef(0);
  const directoryRequestRef = useRef<Record<string, number>>({});
  const searchRequestRef = useRef(0);
  const gitHistoryRequestRef = useRef(0);
  const gitStatusRequestRef = useRef(0);
  const gitDiffRequestRef = useRef(0);

  const trimmedPath = filePath.trim();
  const canUseWorkspace = Boolean(workspaceID);
  const canFetchFileBlob = Boolean(workspaceID && onFetchFileBlob);
  const canSearchWorkspace = Boolean(workspaceID && onSearchWorkspace);
  const gitEnabled = Boolean(onLoadGitStatus && onLoadGitDiff);

  const updateTab = useCallback((tabId: string, patch: Partial<WorkspaceFileTab>) => {
    setTabs((prev) => prev.map((t) => (t.id === tabId ? { ...t, ...patch } : t)));
  }, []);

  const setFilePath = useCallback(
    (path: string) => {
      const id = activeTabIdRef.current;
      if (id) updateTab(id, { filePath: path });
    },
    [updateTab]
  );

  const setFileBody = useCallback(
    (body: string) => {
      const id = activeTabIdRef.current;
      if (id) {
        setTabs((prev) =>
          prev.map((t) => {
            if (t.id !== id) return t;
            if (t.fileBody === body) return t;
            return { ...t, fileBody: body, dirty: true, preview: false };
          })
        );
      }
    },
    []
  );

  const setFileViewMode = useCallback(
    (mode: WorkspaceFileViewMode) => {
      const id = activeTabIdRef.current;
      if (id) updateTab(id, { fileViewMode: mode });
    },
    [updateTab]
  );

  const switchTab = useCallback((tabId: string) => {
    setActiveTabId(tabId);
  }, []);

  const closeTab = useCallback(
    (tabId: string) => {
      const currentTabs = tabsRef.current;
      const index = currentTabs.findIndex((t) => t.id === tabId);
      if (index === -1) return;
      const next = currentTabs.filter((t) => t.id !== tabId);
      setTabs(next);
      if (tabId === activeTabIdRef.current) {
        const newActive =
          next.length === 0
            ? null
            : (next[Math.min(index, next.length - 1)]?.id ?? null);
        setActiveTabId(newActive);
      }
    },
    []
  );

  const closeOtherTabs = useCallback(
    (tabId: string) => {
      setTabs((prev) => prev.filter((t) => t.id === tabId));
      setActiveTabId(tabId);
    },
    []
  );

  const closeAllTabs = useCallback(() => {
    setTabs([]);
    setActiveTabId(null);
  }, []);

  const pinTab = useCallback(
    (tabId: string) => {
      updateTab(tabId, { preview: false });
    },
    [updateTab]
  );

  const reorderTabs = useCallback(
    (fromIndex: number, toIndex: number) => {
      if (fromIndex === toIndex) return;
      setTabs((prev) => {
        if (fromIndex < 0 || fromIndex >= prev.length) return prev;
        const clamped = Math.max(0, Math.min(toIndex, prev.length - 1));
        const next = [...prev];
        const [moved] = next.splice(fromIndex, 1);
        next.splice(clamped, 0, moved);
        return next;
      });
    },
    []
  );

  const setActiveTabEditorViewState = useCallback(
    (viewState: unknown) => {
      const id = activeTabIdRef.current;
      if (id) updateTab(id, { editorViewState: viewState });
    },
    [updateTab]
  );

  const saveTabEditorViewState = useCallback(
    (tabId: string, viewState: unknown) => {
      updateTab(tabId, { editorViewState: viewState });
    },
    [updateTab]
  );

  const saveTabMarkdownPreviewScrollTop = useCallback(
    (tabId: string, scrollTop: number) => {
      if (!Number.isFinite(scrollTop)) return;
      updateTab(tabId, { markdownPreviewScrollTop: Math.max(0, scrollTop) });
    },
    [updateTab]
  );

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

  const clearSearch = useCallback(() => {
    searchRequestRef.current += 1;
    setSearchQuery("");
    setSearchLoading(false);
    setSearchError(null);
    setSearchResults([]);
    setSearchTruncated(false);
    setSearchEngine(undefined);
  }, []);

  const loadSearch = useCallback(
    async (options: { quiet?: boolean } = {}) => {
      const query = searchQuery.trim();
      if (!workspaceID || !onSearchWorkspace || !query) {
        searchRequestRef.current += 1;
        setSearchLoading(false);
        setSearchError(null);
        setSearchResults([]);
        setSearchTruncated(false);
        setSearchEngine(undefined);
        return;
      }
      const requestID = ++searchRequestRef.current;
      setSearchLoading(true);
      setSearchError(null);
      try {
        const response = await onSearchWorkspace(workspaceID, {
          q: query,
          mode: searchMode,
          case_sensitive: searchCaseSensitive,
          regex: searchRegex,
          whole_word: searchWholeWord,
          limit: 200,
        });
        if (searchRequestRef.current !== requestID) return;
        setSearchResults(response.results ?? []);
        setSearchTruncated(response.truncated);
        setSearchEngine(response.engine);
        if (!options.quiet) {
          setWorkspaceStatus(response.truncated ? "Search results limited" : "Search complete");
        }
      } catch (err) {
        if (searchRequestRef.current !== requestID) return;
        const message = err instanceof Error ? err.message : "Search failed";
        setSearchResults([]);
        setSearchTruncated(false);
        setSearchEngine(undefined);
        setSearchError(message);
        if (!options.quiet) setWorkspaceStatus(message);
      } finally {
        if (searchRequestRef.current === requestID) {
          setSearchLoading(false);
        }
      }
    },
    [
      onSearchWorkspace,
      searchCaseSensitive,
      searchMode,
      searchQuery,
      searchRegex,
      searchWholeWord,
      workspaceID,
    ]
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
      setWorkspacePaneViewState("files");
      const isPreview = options.preview === true;
      const normalizedPosition = normalizeWorkspaceFilePosition(options.position);

      const currentTabs = tabsRef.current;
      const currentActiveId = activeTabIdRef.current;
      const normalizedTarget = normalizeWorkspaceTreePath(targetPath);
      const existingTab = currentTabs.find(
        (t) => normalizeWorkspaceTreePath(t.filePath) === normalizedTarget
      );

      if (existingTab) {
        const patch: Partial<WorkspaceFileTab> = {};
        if (normalizedPosition) {
          patch.fileOpenPosition = normalizedPosition;
          patch.fileOpenRequestID = (existingTab.fileOpenRequestID ?? 0) + 1;
        }
        if (!isPreview && existingTab.preview) {
          patch.preview = false;
        }
        if (Object.keys(patch).length > 0) {
          updateTab(existingTab.id, patch);
        }
        setActiveTabId(existingTab.id);
        return;
      }

      const newId = String(++nextTabIdRef.current);
      const newTab: WorkspaceFileTab = {
        id: newId,
        filePath: targetPath,
        fileBody: "",
        fileLoading: true,
        fileLoadError: null,
        editorViewState: null,
        markdownPreviewScrollTop: 0,
        fileViewMode: "edit",
        fileOpenPosition: normalizedPosition,
        fileOpenRequestID: 1,
        dirty: false,
        preview: isPreview,
      };

      if (isPreview) {
        const previewIndex = currentTabs.findIndex((t) => t.preview);
        if (previewIndex !== -1) {
          setTabs((prev) => prev.map((t) => (t.preview ? newTab : t)));
        } else {
          const activeIndex = currentTabs.findIndex((t) => t.id === currentActiveId);
          const insertAt = activeIndex === -1 ? currentTabs.length : activeIndex + 1;
          setTabs((prev) => [...prev.slice(0, insertAt), newTab, ...prev.slice(insertAt)]);
        }
      } else {
        const activeIndex = currentTabs.findIndex((t) => t.id === currentActiveId);
        const insertAt = activeIndex === -1 ? currentTabs.length : activeIndex + 1;
        setTabs((prev) => [...prev.slice(0, insertAt), newTab, ...prev.slice(insertAt)]);
      }
      setActiveTabId(newId);

      const requestID = (tabFileRequestRef.current[newId] ?? 0) + 1;
      tabFileRequestRef.current[newId] = requestID;

      try {
        if (isPdfFilePath(targetPath)) {
          if (tabFileRequestRef.current[newId] !== requestID) return;
          updateTab(newId, { fileBody: "", fileLoadError: null, fileLoading: false });
          setWorkspaceStatus("Loaded");
          return;
        }
        const body = await onReadFile(workspaceID, targetPath);
        if (tabFileRequestRef.current[newId] !== requestID) return;
        updateTab(newId, { fileBody: body, fileLoadError: null, fileLoading: false });
        setWorkspaceStatus("Loaded");
      } catch (err) {
        if (tabFileRequestRef.current[newId] !== requestID) return;
        const message = err instanceof Error ? err.message : "Load failed";
        updateTab(newId, { fileBody: "", fileLoadError: message, fileLoading: false });
        setWorkspaceStatus(message);
      }
    },
    [filePath, onReadFile, updateTab, workspaceID]
  );

  const loadGitStatus = useCallback(
    async (options: { quiet?: boolean; scope?: WorkspaceGitScope; target?: string; compare?: string; commit?: string } = {}) => {
      const targetScope = options.scope ?? gitScope;
      const targetBranch = options.target ?? gitTarget;
      const compareBranch = options.compare ?? gitCompare;
      const targetCommit = options.commit ?? gitSelectedCommit;
      if (!workspaceID || !onLoadGitStatus) return;
      if (targetScope === "commit" && !targetCommit) {
        gitStatusRequestRef.current += 1;
        setGitStatus(undefined);
        setGitStatusLoading(false);
        setGitStatusError(null);
        return;
      }
      const requestID = ++gitStatusRequestRef.current;
      setGitStatusLoading(true);
      setGitStatusError(null);
      try {
        const status = await onLoadGitStatus(
          workspaceID,
          targetScope,
          targetScope === "branch" ? targetBranch : undefined,
          targetScope === "branch" ? compareBranch : undefined,
          targetScope === "commit" ? targetCommit : undefined
        );
        if (gitStatusRequestRef.current !== requestID) return;
        setGitStatus(status);
        if (targetScope === "branch") {
          setGitTargetState(status.target ?? targetBranch);
          setGitCompareState(status.compare ?? status.branch ?? compareBranch);
        }
        if (!options.quiet) setWorkspaceStatus(status.available ? "Changes loaded" : status.message ?? "Git unavailable");
      } catch (err) {
        if (gitStatusRequestRef.current !== requestID) return;
        const message = err instanceof Error ? err.message : "Changes load failed";
        setGitStatus(undefined);
        setGitStatusError(message);
        if (!options.quiet) setWorkspaceStatus(message);
      } finally {
        if (gitStatusRequestRef.current === requestID) {
          setGitStatusLoading(false);
        }
      }
    },
    [gitCompare, gitScope, gitSelectedCommit, gitTarget, onLoadGitStatus, workspaceID]
  );

  const loadGitHistory = useCallback(
    async (options: { quiet?: boolean; mode?: WorkspaceGitHistoryMode; offset?: number; append?: boolean } = {}) => {
      const mode = options.mode ?? gitHistoryMode;
      const path = mode === "file" ? normalizeWorkspaceTreePath(trimmedPath) : "";
      const query = gitHistoryQuery.trim();
      if (!workspaceID || !onLoadGitHistory) return;
      if (mode === "file" && !path) {
        gitHistoryRequestRef.current += 1;
        setGitHistory({
          available: true,
          mode,
          path: "",
          limit: 50,
          offset: 0,
          has_more: false,
          message: "Open a file to view its history.",
          commits: [],
        });
        setGitHistoryLoading(false);
        setGitHistoryError(null);
        return;
      }
      const requestID = ++gitHistoryRequestRef.current;
      const offset = options.offset ?? 0;
      setGitHistoryLoading(true);
      setGitHistoryError(null);
      try {
        const history = await onLoadGitHistory(workspaceID, {
          mode,
          path: mode === "file" ? path : undefined,
          q: query || undefined,
          limit: 50,
          offset,
        });
        if (gitHistoryRequestRef.current !== requestID) return;
        setGitHistory((current) => {
          if (!options.append) return history;
          return {
            ...history,
            commits: [...(current?.commits ?? []), ...(history.commits ?? [])],
          };
        });
        if (!options.quiet) {
          setWorkspaceStatus(history.available ? "History loaded" : history.message ?? "Git history unavailable");
        }
      } catch (err) {
        if (gitHistoryRequestRef.current !== requestID) return;
        const message = err instanceof Error ? err.message : "History load failed";
        if (!options.append) setGitHistory(undefined);
        setGitHistoryError(message);
        if (!options.quiet) setWorkspaceStatus(message);
      } finally {
        if (gitHistoryRequestRef.current === requestID) {
          setGitHistoryLoading(false);
        }
      }
    },
    [gitHistoryMode, gitHistoryQuery, onLoadGitHistory, trimmedPath, workspaceID]
  );

  const selectGitCommit = useCallback(
    async (commit: WorkspaceGitCommit) => {
      setGitSelectedCommit(commit.sha);
      setGitDiff(undefined);
      setGitDiffError(null);
      setGitSelectedPath("");
      await loadGitStatus({ quiet: true, scope: "commit", commit: commit.sha });
    },
    [loadGitStatus]
  );

  const loadGitDiff = useCallback(
    async (path: string) => {
      const targetPath = path.trim();
      if (!workspaceID || !onLoadGitDiff || !targetPath) return;
      setWorkspacePaneViewState("changes");
      setGitSelectedPath(targetPath);
      const requestID = ++gitDiffRequestRef.current;
      setGitDiff(undefined);
      setGitDiffLoading(true);
      setGitDiffError(null);
      try {
        const diff = await onLoadGitDiff(
          workspaceID,
          gitScope,
          targetPath,
          gitScope === "branch" ? gitTarget : undefined,
          gitScope === "branch" ? gitCompare : undefined,
          gitScope === "commit" ? gitSelectedCommit : undefined
        );
        if (gitDiffRequestRef.current !== requestID) return;
        setGitDiff(diff);
        setWorkspaceStatus("Diff loaded");
      } catch (err) {
        if (gitDiffRequestRef.current !== requestID) return;
        const message = err instanceof Error ? err.message : "Diff load failed";
        setGitDiff(undefined);
        setGitDiffError(message);
        setWorkspaceStatus(message);
      } finally {
        if (gitDiffRequestRef.current === requestID) {
          setGitDiffLoading(false);
        }
      }
    },
    [gitCompare, gitScope, gitSelectedCommit, gitTarget, onLoadGitDiff, workspaceID]
  );

  const setWorkspacePaneView = useCallback(
    (view: WorkspacePaneView) => {
      setWorkspacePaneViewState(view);
      if (view === "changes") {
        if (gitScope === "commit") {
          void loadGitHistory({ quiet: true });
        } else {
          void loadGitStatus({ quiet: true });
        }
      }
    },
    [gitScope, loadGitHistory, loadGitStatus]
  );

  const setGitScope = useCallback(
    (scope: WorkspaceGitScope) => {
      setGitScopeState(scope);
      setGitDiff(undefined);
      setGitDiffError(null);
      setGitSelectedPath("");
      if (workspacePaneView === "changes") {
        if (scope === "commit") {
          void loadGitHistory({ quiet: true });
        } else {
          void loadGitStatus({ quiet: true, scope, target: gitTarget, compare: gitCompare });
        }
      }
    },
    [gitCompare, gitTarget, loadGitHistory, loadGitStatus, workspacePaneView]
  );

  const setGitTarget = useCallback(
    (target: string) => {
      setGitTargetState(target);
      setGitDiff(undefined);
      setGitDiffError(null);
      setGitSelectedPath("");
      if (workspacePaneView === "changes" && gitScope === "branch") {
        void loadGitStatus({ quiet: true, scope: "branch", target, compare: gitCompare });
      }
    },
    [gitCompare, gitScope, loadGitStatus, workspacePaneView]
  );

  const setGitCompare = useCallback(
    (compare: string) => {
      setGitCompareState(compare);
      setGitDiff(undefined);
      setGitDiffError(null);
      setGitSelectedPath("");
      if (workspacePaneView === "changes" && gitScope === "branch") {
        void loadGitStatus({ quiet: true, scope: "branch", target: gitTarget, compare });
      }
    },
    [gitScope, gitTarget, loadGitStatus, workspacePaneView]
  );

  const setGitHistoryMode = useCallback(
    (mode: WorkspaceGitHistoryMode) => {
      gitHistoryRequestRef.current += 1;
      setGitHistoryModeState(mode);
      setGitHistory(undefined);
      setGitHistoryLoading(false);
      setGitHistoryError(null);
      setGitSelectedCommit("");
      setGitStatus(undefined);
      setGitDiff(undefined);
      setGitDiffError(null);
      setGitSelectedPath("");
      if (workspacePaneView === "changes" && gitScope === "commit") {
        void loadGitHistory({ quiet: true, mode });
      }
    },
    [gitScope, loadGitHistory, workspacePaneView]
  );

  const setGitHistoryQuery = useCallback(
    (query: string) => {
      gitHistoryRequestRef.current += 1;
      setGitHistoryQueryState(query);
      setGitHistory(undefined);
      setGitHistoryLoading(false);
      setGitHistoryError(null);
      setGitSelectedCommit("");
      setGitStatus(undefined);
      setGitDiff(undefined);
      setGitDiffError(null);
      setGitSelectedPath("");
    },
    []
  );

  const saveFile = useCallback(async () => {
    const targetPath = filePath.trim();
    if (!workspaceID || !targetPath || !activeTabId) return;
    if (isPdfFilePath(targetPath)) {
      setWorkspaceStatus("PDF files cannot be edited here");
      return;
    }
    setFileSaving(true);
    try {
      await onWriteFile(workspaceID, targetPath, fileBody);
      updateTab(activeTabId, { filePath: targetPath, fileLoadError: null, dirty: false, preview: false });
      setWorkspaceStatus("Saved");
      await refreshTreeForFilePath(targetPath);
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Save failed");
    } finally {
      setFileSaving(false);
    }
  }, [activeTabId, fileBody, filePath, onWriteFile, refreshTreeForFilePath, updateTab, workspaceID]);

  const fetchFileBlob = useCallback(
    async (path = filePath, options: { download?: boolean } = {}) => {
      const targetPath = path.trim();
      if (!workspaceID || !targetPath || !onFetchFileBlob) {
        throw new Error("File content is not available");
      }
      return onFetchFileBlob(workspaceID, targetPath, options);
    },
    [filePath, onFetchFileBlob, workspaceID]
  );

  const downloadFile = useCallback(async () => {
    const targetPath = filePath.trim();
    if (!workspaceID || !targetPath || !onFetchFileBlob) return;
    setFileDownloading(true);
    try {
      const blob = await onFetchFileBlob(workspaceID, targetPath, { download: true });
      downloadWorkspaceFileBlob(blob, workspaceFileDownloadName(targetPath));
      setWorkspaceStatus("Downloaded");
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Download failed");
    } finally {
      setFileDownloading(false);
    }
  }, [filePath, onFetchFileBlob, workspaceID]);

  const deleteFile = useCallback(async () => {
    const targetPath = filePath.trim();
    if (!workspaceID || !targetPath || !activeTabId) return;
    setFileDeleting(true);
    try {
      await onDeleteFile(workspaceID, targetPath);
      closeTab(activeTabId);
      setWorkspaceStatus("Deleted");
      await refreshTreeForFilePath(targetPath);
    } catch (err) {
      setWorkspaceStatus(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setFileDeleting(false);
    }
  }, [activeTabId, closeTab, filePath, onDeleteFile, refreshTreeForFilePath, workspaceID]);

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
          const newId = String(++nextTabIdRef.current);
          const newTab: WorkspaceFileTab = {
            id: newId,
            filePath: targetPath,
            fileBody: "",
            fileLoading: false,
            fileLoadError: null,
            editorViewState: null,
            markdownPreviewScrollTop: 0,
            fileViewMode: "edit",
            fileOpenPosition: undefined,
            fileOpenRequestID: 1,
            dirty: false,
            preview: false,
          };
          const currentActiveId = activeTabIdRef.current;
          setTabs((prev) => {
            const activeIndex = prev.findIndex((t) => t.id === currentActiveId);
            const insertAt = activeIndex === -1 ? prev.length : activeIndex + 1;
            return [...prev.slice(0, insertAt), newTab, ...prev.slice(insertAt)];
          });
          setActiveTabId(newId);
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
        setTabs((prev) =>
          prev.map((t) => {
            const newPath = replaceMovedWorkspacePath(t.filePath, entry.path, targetPath);
            return newPath !== t.filePath ? { ...t, filePath: newPath } : t;
          })
        );
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
        setTabs((prev) => {
          const next = prev.filter(
            (t) => !workspacePathIsSameOrInside(t.filePath.trim(), entry.path)
          );
          if (next.length !== prev.length && activeTabId) {
            const activeRemoved = !next.some((t) => t.id === activeTabId);
            if (activeRemoved) {
              const oldIndex = prev.findIndex((t) => t.id === activeTabId);
              const newActive =
                next.length === 0
                  ? null
                  : (next[Math.min(oldIndex, next.length - 1)]?.id ?? null);
              setActiveTabId(newActive);
            }
          }
          return next;
        });
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
    [activeTabId, filePath, onDeleteEntry, onDeleteFile, refreshTreeForEntryPaths, workspaceID]
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
        setTabs((prev) =>
          prev.map((t) => {
            const newPath = replaceMovedWorkspacePath(t.filePath, entry.path, targetPath);
            return newPath !== t.filePath ? { ...t, filePath: newPath } : t;
          })
        );
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
    searchRequestRef.current += 1;
    tabFileRequestRef.current = {};
    gitHistoryRequestRef.current += 1;
    gitStatusRequestRef.current += 1;
    gitDiffRequestRef.current += 1;
    setTabs([]);
    setActiveTabId(null);
    setTree(undefined);
    setWorkspaceTreeResetKey((current) => current + 1);
    setWorkspaceTreeError(null);
    setDirectoryLoadingPaths(new Set());
    setDirectoryLoadErrors({});
    setSearchQuery("");
    setSearchMode("files");
    setSearchCaseSensitive(false);
    setSearchRegex(false);
    setSearchWholeWord(false);
    setSearchLoading(false);
    setSearchError(null);
    setSearchResults([]);
    setSearchTruncated(false);
    setSearchEngine(undefined);
    setWorkspaceTreeLoading(false);
    setFileSaving(false);
    setFileDownloading(false);
    setFileDeleting(false);
    setEntryActionPending(false);
    setWorkspacePaneViewState("files");
    setGitScopeState("working_tree");
    setGitTargetState("");
    setGitCompareState("");
    setGitHistoryModeState("repository");
    setGitHistoryQueryState("");
    setGitHistory(undefined);
    setGitHistoryLoading(false);
    setGitHistoryError(null);
    setGitSelectedCommit("");
    setGitStatus(undefined);
    setGitStatusLoading(false);
    setGitStatusError(null);
    setGitDiff(undefined);
    setGitDiffLoading(false);
    setGitDiffError(null);
    setGitSelectedPath("");
    setWorkspaceStatus(null);
  }, [initialPath, workspaceID]);

  useEffect(() => {
    if (autoLoadTree && workspaceID) {
      void loadTree({ quiet: true });
    }
  }, [autoLoadTree, initialPath, loadTree, workspaceID]);

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
    searchQuery,
    searchMode,
    searchCaseSensitive,
    searchRegex,
    searchWholeWord,
    searchLoading,
    searchError,
    searchResults,
    searchTruncated,
    searchEngine,
    fileLoading,
    fileLoadError,
    fileSaving,
    fileDownloading,
    fileDeleting,
    entryActionPending,
    workspaceStatus,
    workspacePaneView,
    gitEnabled,
    gitScope,
    gitTarget,
    gitCompare,
    gitHistoryMode,
    gitHistoryQuery,
    gitHistory,
    gitHistoryLoading,
    gitHistoryError,
    gitSelectedCommit,
    gitStatus,
    gitStatusLoading,
    gitStatusError,
    gitDiff,
    gitDiffLoading,
    gitDiffError,
    gitSelectedPath,
    fileOpenPosition,
    fileOpenRequestID,
    fileViewMode,
    trimmedPath,
    canUseWorkspace,
    canFetchFileBlob,
    canSearchWorkspace,
    tabs,
    activeTabId,
    activeTabEditorViewState: activeTab?.editorViewState ?? null,
    activeTabMarkdownPreviewScrollTop: activeTab?.markdownPreviewScrollTop ?? 0,
    switchTab,
    closeTab,
    closeOtherTabs,
    closeAllTabs,
    pinTab,
    reorderTabs,
    setActiveTabEditorViewState,
    saveTabEditorViewState,
    saveTabMarkdownPreviewScrollTop,
    setFilePath,
    setFileBody,
    setSearchQuery,
    setSearchMode,
    setSearchCaseSensitive,
    setSearchRegex,
    setSearchWholeWord,
    setFileViewMode,
    setWorkspacePaneView,
    setGitScope,
    setGitTarget,
    setGitCompare,
    setGitHistoryMode,
    setGitHistoryQuery,
    loadTree,
    loadDirectory,
    loadSearch,
    clearSearch,
    loadFile,
    loadGitStatus,
    loadGitHistory,
    selectGitCommit,
    loadGitDiff,
    saveFile,
    fetchFileBlob,
    downloadFile,
    deleteFile,
    createEntry,
    renameEntry,
    deleteEntry,
    moveEntry,
  };
}
