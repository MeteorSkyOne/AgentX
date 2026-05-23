import {
  createWorkspaceEntry,
  deleteWorkspaceEntry,
  deleteWorkspaceFile,
  fetchWorkspaceFileBlob,
  moveWorkspaceEntry,
  putWorkspaceFile,
  workspaceFile,
  workspaceGitDiff,
  workspaceGitHistory,
  workspaceGitStatus,
  workspaceSearch,
  workspaceTree,
} from "@/api/client";
import type {
  WorkspaceEntryType,
  WorkspaceGitDiff,
  WorkspaceGitHistory,
  WorkspaceGitHistoryMode,
  WorkspaceGitScope,
  WorkspaceGitStatus,
  WorkspaceSearchMode,
  WorkspaceSearchResponse,
  WorkspaceTreeEntry,
} from "@/api/types";

export async function handleReadWorkspaceFile(workspaceID: string, path: string): Promise<string> {
    const file = await workspaceFile(workspaceID, path);
    return file.body;
  }

export async function handleFetchWorkspaceFileBlob(
    workspaceID: string,
    path: string,
    options?: { download?: boolean }
  ): Promise<Blob> {
    return fetchWorkspaceFileBlob(workspaceID, path, options);
  }

export async function handleLoadWorkspaceTree(workspaceID: string, path?: string): Promise<WorkspaceTreeEntry> {
    return workspaceTree(workspaceID, path);
  }

export async function handleSearchWorkspace(
    workspaceID: string,
    options: {
      q: string;
      mode?: WorkspaceSearchMode;
      case_sensitive?: boolean;
      regex?: boolean;
      whole_word?: boolean;
      limit?: number;
    }
  ): Promise<WorkspaceSearchResponse> {
    return workspaceSearch(workspaceID, options);
  }

export async function handleWriteWorkspaceFile(workspaceID: string, path: string, body: string) {
    await putWorkspaceFile(workspaceID, path, body);
  }

export async function handleDeleteWorkspaceFile(workspaceID: string, path: string) {
    await deleteWorkspaceFile(workspaceID, path);
  }

export async function handleCreateWorkspaceEntry(
    workspaceID: string,
    path: string,
    type: WorkspaceEntryType
  ) {
    await createWorkspaceEntry(workspaceID, path, type);
  }

export async function handleMoveWorkspaceEntry(workspaceID: string, path: string, newPath: string) {
    await moveWorkspaceEntry(workspaceID, path, newPath);
  }

export async function handleDeleteWorkspaceEntry(workspaceID: string, path: string) {
    await deleteWorkspaceEntry(workspaceID, path);
  }

export async function handleLoadWorkspaceGitStatus(
    workspaceID: string,
    scope: WorkspaceGitScope,
    target?: string,
    compare?: string,
    commit?: string
  ): Promise<WorkspaceGitStatus> {
    return workspaceGitStatus(workspaceID, scope, target, compare, commit);
  }

export async function handleLoadWorkspaceGitHistory(
    workspaceID: string,
    options: {
      mode: WorkspaceGitHistoryMode;
      path?: string;
      q?: string;
      limit?: number;
      offset?: number;
    }
  ): Promise<WorkspaceGitHistory> {
    return workspaceGitHistory(workspaceID, options);
  }

export async function handleLoadWorkspaceGitDiff(
    workspaceID: string,
    scope: WorkspaceGitScope,
    path: string,
    target?: string,
    compare?: string,
    commit?: string
  ): Promise<WorkspaceGitDiff> {
    return workspaceGitDiff(workspaceID, scope, path, target, compare, commit);
  }
