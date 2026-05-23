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
import type { ThemeMode } from "@/theme";

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

export interface WorkspaceFileBrowserProps extends WorkspaceFileBrowserActions {
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
