import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import {
  ChevronDown,
  ChevronUp,
  Code2,
  Columns2,
  CaseSensitive,
  Database,
  Download,
  Eye,
  FileDiff,
  FileSearch,
  FileText,
  FolderOpen,
  GitBranch,
  GitCompare,
  History,
  Regex,
  RefreshCw,
  Save,
  Search,
  TextSearch,
  Trash2,
  WholeWord,
  X,
} from "lucide-react";
import type {
  WorkspaceEntryType,
  WorkspaceGitChange,
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
import { Select } from "@/components/ui/select";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { FileTree } from "./FileTree";
import { isMarkdownFilePath, isPdfFilePath } from "./workspaceFileLanguages";

const LazyWorkspaceFileEditor = lazy(() =>
  import("./WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceFileEditor }))
);

const LazyWorkspaceGitDiffViewer = lazy(() =>
  import("./WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceGitDiffViewer }))
);

const workspaceSearchDebounceMs = 400;

export interface WorkspaceFileBrowserActions {
  onLoadTree: (workspaceID: string, path?: string) => Promise<WorkspaceTreeEntry>;
  onSearchWorkspace?: (
    workspaceID: string,
    options: {
      q: string;
      mode?: WorkspaceSearchMode;
      case_sensitive?: boolean;
      regex?: boolean;
      whole_word?: boolean;
      limit?: number;
    }
  ) => Promise<WorkspaceSearchResponse>;
  onReadFile: (workspaceID: string, path: string) => Promise<string>;
  onFetchFileBlob?: (
    workspaceID: string,
    path: string,
    options?: { download?: boolean }
  ) => Promise<Blob>;
  onWriteFile: (workspaceID: string, path: string, body: string) => Promise<void>;
  onDeleteFile: (workspaceID: string, path: string) => Promise<void>;
  onCreateEntry?: (workspaceID: string, path: string, type: WorkspaceEntryType) => Promise<void>;
  onMoveEntry?: (workspaceID: string, path: string, newPath: string) => Promise<void>;
  onDeleteEntry?: (workspaceID: string, path: string) => Promise<void>;
  onLoadGitStatus?: (
    workspaceID: string,
    scope: WorkspaceGitScope,
    target?: string,
    compare?: string,
    commit?: string
  ) => Promise<WorkspaceGitStatus>;
  onLoadGitHistory?: (
    workspaceID: string,
    options: {
      mode: WorkspaceGitHistoryMode;
      path?: string;
      q?: string;
      limit?: number;
      offset?: number;
    }
  ) => Promise<WorkspaceGitHistory>;
  onLoadGitDiff?: (
    workspaceID: string,
    scope: WorkspaceGitScope,
    path: string,
    target?: string,
    compare?: string,
    commit?: string
  ) => Promise<WorkspaceGitDiff>;
}

export interface WorkspaceFilePosition {
  lineNumber: number;
  column?: number;
}

export interface WorkspaceFileOpenOptions {
  position?: WorkspaceFilePosition;
  preview?: boolean;
}

export type WorkspaceFileViewMode = "edit" | "preview" | "split";
export type WorkspacePaneView = "files" | "changes";

export interface WorkspaceFileTab {
  id: string;
  filePath: string;
  fileBody: string;
  fileLoading: boolean;
  fileLoadError: string | null;
  editorViewState: unknown;
  markdownPreviewScrollTop: number;
  fileViewMode: WorkspaceFileViewMode;
  fileOpenPosition?: WorkspaceFilePosition;
  fileOpenRequestID: number;
  dirty: boolean;
  preview: boolean;
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
  workspaceTreeResetKey: number;
  workspaceTreeLoading: boolean;
  workspaceTreeError: string | null;
  directoryLoadingPaths: ReadonlySet<string>;
  directoryLoadErrors: Record<string, string | null | undefined>;
  searchQuery: string;
  searchMode: WorkspaceSearchMode;
  searchCaseSensitive: boolean;
  searchRegex: boolean;
  searchWholeWord: boolean;
  searchLoading: boolean;
  searchError: string | null;
  searchResults: WorkspaceSearchResult[];
  searchTruncated: boolean;
  searchEngine?: WorkspaceSearchResponse["engine"];
  fileLoading: boolean;
  fileLoadError: string | null;
  fileSaving: boolean;
  fileDownloading: boolean;
  fileDeleting: boolean;
  entryActionPending: boolean;
  workspaceStatus: string | null;
  workspacePaneView: WorkspacePaneView;
  gitEnabled: boolean;
  gitScope: WorkspaceGitScope;
  gitTarget: string;
  gitCompare: string;
  gitHistoryMode: WorkspaceGitHistoryMode;
  gitHistoryQuery: string;
  gitHistory?: WorkspaceGitHistory;
  gitHistoryLoading: boolean;
  gitHistoryError: string | null;
  gitSelectedCommit: string;
  gitStatus?: WorkspaceGitStatus;
  gitStatusLoading: boolean;
  gitStatusError: string | null;
  gitDiff?: WorkspaceGitDiff;
  gitDiffLoading: boolean;
  gitDiffError: string | null;
  gitSelectedPath: string;
  fileOpenPosition?: WorkspaceFilePosition;
  fileOpenRequestID: number;
  fileViewMode: WorkspaceFileViewMode;
  trimmedPath: string;
  canUseWorkspace: boolean;
  canFetchFileBlob: boolean;
  canSearchWorkspace: boolean;
  tabs: readonly WorkspaceFileTab[];
  activeTabId: string | null;
  activeTabEditorViewState: unknown;
  activeTabMarkdownPreviewScrollTop: number;
  switchTab: (tabId: string) => void;
  closeTab: (tabId: string) => void;
  closeOtherTabs: (tabId: string) => void;
  closeAllTabs: () => void;
  pinTab: (tabId: string) => void;
  reorderTabs: (fromIndex: number, toIndex: number) => void;
  setActiveTabEditorViewState: (viewState: unknown) => void;
  saveTabEditorViewState: (tabId: string, viewState: unknown) => void;
  saveTabMarkdownPreviewScrollTop: (tabId: string, scrollTop: number) => void;
  setFilePath: (path: string) => void;
  setFileBody: (body: string) => void;
  setSearchQuery: (query: string) => void;
  setSearchMode: (mode: WorkspaceSearchMode) => void;
  setSearchCaseSensitive: (caseSensitive: boolean) => void;
  setSearchRegex: (regex: boolean) => void;
  setSearchWholeWord: (wholeWord: boolean) => void;
  setFileViewMode: (mode: WorkspaceFileViewMode) => void;
  setWorkspacePaneView: (view: WorkspacePaneView) => void;
  setGitScope: (scope: WorkspaceGitScope) => void;
  setGitTarget: (target: string) => void;
  setGitCompare: (compare: string) => void;
  setGitHistoryMode: (mode: WorkspaceGitHistoryMode) => void;
  setGitHistoryQuery: (query: string) => void;
  loadTree: (options?: { quiet?: boolean }) => Promise<void>;
  loadDirectory: (path: string, options?: { quiet?: boolean; force?: boolean }) => Promise<void>;
  loadSearch: (options?: { quiet?: boolean }) => Promise<void>;
  clearSearch: () => void;
  loadFile: (path?: string, options?: WorkspaceFileOpenOptions) => Promise<void>;
  loadGitStatus: (options?: { quiet?: boolean; scope?: WorkspaceGitScope; target?: string; compare?: string; commit?: string }) => Promise<void>;
  loadGitHistory: (options?: { quiet?: boolean; mode?: WorkspaceGitHistoryMode; offset?: number; append?: boolean }) => Promise<void>;
  selectGitCommit: (commit: WorkspaceGitCommit) => Promise<void>;
  loadGitDiff: (path: string) => Promise<void>;
  saveFile: () => Promise<void>;
  fetchFileBlob: (path?: string, options?: { download?: boolean }) => Promise<Blob>;
  downloadFile: () => Promise<void>;
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

function normalizeWorkspaceFilePosition(
  position?: WorkspaceFilePosition
): WorkspaceFilePosition | undefined {
  if (!position || position.lineNumber < 1) return undefined;
  return {
    lineNumber: position.lineNumber,
    column: position.column && position.column > 0 ? position.column : 1,
  };
}

function downloadWorkspaceFileBlob(blob: Blob, filename: string) {
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

function workspaceFileDownloadName(path: string): string {
  return path.trim().split(/[\\/]/).pop() || "download";
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

function gitChangeCode(change: WorkspaceGitChange): string {
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

function gitChangeStatusLabel(status: WorkspaceGitChange["status"]): string {
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

function gitChangeTone(status: WorkspaceGitChange["status"]): string {
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

function WorkspaceFileTabBar({
  controller,
}: {
  controller: WorkspaceFileBrowserController;
}) {
  const [dragTabId, setDragTabId] = useState<string | null>(null);
  const [dropIndicator, setDropIndicator] = useState<{ index: number; side: "left" | "right" } | null>(null);

  if (controller.tabs.length === 0) return null;

  function handleDragStart(e: React.DragEvent, tabId: string) {
    setDragTabId(tabId);
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("application/x-agentx-tab", tabId);
  }

  function handleDragOver(e: React.DragEvent, index: number) {
    if (!dragTabId) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    const rect = e.currentTarget.getBoundingClientRect();
    const midX = rect.left + rect.width / 2;
    const side = e.clientX < midX ? "left" : "right";
    setDropIndicator({ index, side });
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    if (!dragTabId || !dropIndicator) return;
    const fromIndex = controller.tabs.findIndex((t) => t.id === dragTabId);
    if (fromIndex === -1) return;
    let toIndex = dropIndicator.side === "right" ? dropIndicator.index + 1 : dropIndicator.index;
    if (fromIndex < toIndex) toIndex -= 1;
    controller.reorderTabs(fromIndex, toIndex);
    setDragTabId(null);
    setDropIndicator(null);
  }

  function handleDragEnd() {
    setDragTabId(null);
    setDropIndicator(null);
  }

  return (
    <div
      className="flex shrink-0 items-center overflow-x-auto border-b border-border bg-muted/30"
      role="tablist"
      aria-label="Open files"
      style={{ scrollbarWidth: "none" }}
      onDrop={handleDrop}
      onDragOver={(e) => { if (dragTabId) e.preventDefault(); }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setDropIndicator(null);
        }
      }}
    >
      {controller.tabs.map((tab, index) => {
        const active = tab.id === controller.activeTabId;
        const dragging = tab.id === dragTabId;
        const fileName = tab.filePath.split("/").pop() || tab.filePath;
        const showLeftIndicator = dropIndicator?.index === index && dropIndicator.side === "left";
        const showRightIndicator = dropIndicator?.index === index && dropIndicator.side === "right";

        return (
          <ContextMenu key={tab.id}>
            <ContextMenuTrigger asChild>
              <button
                type="button"
                role="tab"
                aria-selected={active}
                title={tab.filePath}
                draggable
                onDragStart={(e) => handleDragStart(e, tab.id)}
                onDragEnd={handleDragEnd}
                onDragOver={(e) => handleDragOver(e, index)}
                className={cn(
                  "group relative flex shrink-0 items-center gap-1.5 border-r border-border px-3 py-1.5 text-xs outline-none transition-colors",
                  active
                    ? "bg-background text-foreground"
                    : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
                  dragging && "opacity-40"
                )}
                onClick={() => controller.switchTab(tab.id)}
                onDoubleClick={() => {
                  if (tab.preview) controller.pinTab(tab.id);
                }}
                onMouseDown={(e) => {
                  if (e.button === 1) {
                    e.preventDefault();
                    controller.closeTab(tab.id);
                  }
                }}
              >
                {showLeftIndicator && (
                  <span className="pointer-events-none absolute -left-px top-1 bottom-1 w-0.5 rounded-full bg-primary" />
                )}
                <FileText className="h-3 w-3 shrink-0 text-blue-400" />
                <span className={cn("max-w-40 truncate", tab.preview && "italic")}>
                  {fileName}
                </span>
                {tab.dirty ? (
                  <span
                    className={cn(
                      "ml-0.5 inline-block h-2 w-2 shrink-0 rounded-full bg-current",
                      "group-hover:hidden"
                    )}
                    aria-label="Unsaved changes"
                  />
                ) : null}
                <button
                  type="button"
                  className={cn(
                    "ml-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-sm hover:bg-accent",
                    !tab.dirty && "opacity-0 group-hover:opacity-100",
                    tab.dirty && "hidden group-hover:inline-flex"
                  )}
                  title="Close"
                  aria-label={`Close ${fileName}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    controller.closeTab(tab.id);
                  }}
                  draggable={false}
                >
                  <X className="h-3 w-3" />
                </button>
                {showRightIndicator && (
                  <span className="pointer-events-none absolute -right-px top-1 bottom-1 w-0.5 rounded-full bg-primary" />
                )}
              </button>
            </ContextMenuTrigger>
            <ContextMenuContent>
              <ContextMenuItem onSelect={() => controller.closeTab(tab.id)}>
                Close
              </ContextMenuItem>
              <ContextMenuItem onSelect={() => controller.closeOtherTabs(tab.id)}>
                Close Others
              </ContextMenuItem>
              <ContextMenuItem onSelect={() => controller.closeAllTabs()}>
                Close All
              </ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuItem
                onSelect={() => {
                  void navigator.clipboard.writeText(tab.filePath);
                }}
              >
                Copy Path
              </ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        );
      })}
    </div>
  );
}

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
                <div className="flex h-full flex-col">
                  <WorkspaceFileTabBar controller={controller} />
                  <WorkspaceFileEditor
                    controller={controller}
                    theme={theme}
                    contentAriaLabel="File content"
                    className="min-h-0 flex-1"
                  />
                </div>
              </ResizablePanel>
            </ResizablePanelGroup>
          ) : (
            <div className="flex h-full flex-col">
              <WorkspaceFileTabBar controller={controller} />
              <WorkspaceFileEditor
                controller={controller}
                theme={theme}
                contentAriaLabel="File content"
                className="min-h-0 flex-1"
              />
            </div>
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
