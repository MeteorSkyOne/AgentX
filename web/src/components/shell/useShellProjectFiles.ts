import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import type { Workspace } from "@/api/types";
import { useWorkspaceFileBrowser } from "../WorkspaceFileBrowser";
import type { ShellProps } from "./types";
import { blurActiveElement } from "./utils";

interface UseShellProjectFilesArgs {
  projectID?: string;
  projectWorkspace?: Workspace;
  setMobileNavOpen: Dispatch<SetStateAction<boolean>>;
  setMobileAgentPanelOpen: Dispatch<SetStateAction<boolean>>;
  setMobileMembersPanelOpen: Dispatch<SetStateAction<boolean>>;
  onLoadWorkspaceTree: ShellProps["onLoadWorkspaceTree"];
  onSearchWorkspace: ShellProps["onSearchWorkspace"];
  onReadWorkspaceFile: ShellProps["onReadWorkspaceFile"];
  onFetchWorkspaceFileBlob: ShellProps["onFetchWorkspaceFileBlob"];
  onWriteWorkspaceFile: ShellProps["onWriteWorkspaceFile"];
  onDeleteWorkspaceFile: ShellProps["onDeleteWorkspaceFile"];
  onCreateWorkspaceEntry: ShellProps["onCreateWorkspaceEntry"];
  onMoveWorkspaceEntry: ShellProps["onMoveWorkspaceEntry"];
  onDeleteWorkspaceEntry: ShellProps["onDeleteWorkspaceEntry"];
  onLoadWorkspaceGitStatus: ShellProps["onLoadWorkspaceGitStatus"];
  onLoadWorkspaceGitHistory: ShellProps["onLoadWorkspaceGitHistory"];
  onLoadWorkspaceGitDiff: ShellProps["onLoadWorkspaceGitDiff"];
}

export function useShellProjectFiles({
  projectID,
  projectWorkspace,
  setMobileNavOpen,
  setMobileAgentPanelOpen,
  setMobileMembersPanelOpen,
  onLoadWorkspaceTree,
  onSearchWorkspace,
  onReadWorkspaceFile,
  onFetchWorkspaceFileBlob,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onCreateWorkspaceEntry,
  onMoveWorkspaceEntry,
  onDeleteWorkspaceEntry,
  onLoadWorkspaceGitStatus,
  onLoadWorkspaceGitHistory,
  onLoadWorkspaceGitDiff,
}: UseShellProjectFilesArgs) {
  const [projectFilesOpen, setProjectFilesOpen] = useState(false);
  const [projectFileTreeCollapsed, setProjectFileTreeCollapsed] = useState(false);
  const [mobileProjectFilesView, setMobileProjectFilesView] = useState<"tree" | "editor">("tree");
  const [mobileEditorHeaderCollapsed, setMobileEditorHeaderCollapsed] = useState(false);
  const controller = useWorkspaceFileBrowser({
    workspaceID: projectWorkspace?.id,
    workspacePath: projectWorkspace?.path,
    autoLoadTree: false,
    onLoadTree: onLoadWorkspaceTree,
    onSearchWorkspace,
    onReadFile: onReadWorkspaceFile,
    onFetchFileBlob: onFetchWorkspaceFileBlob,
    onWriteFile: onWriteWorkspaceFile,
    onDeleteFile: onDeleteWorkspaceFile,
    onCreateEntry: onCreateWorkspaceEntry,
    onMoveEntry: onMoveWorkspaceEntry,
    onDeleteEntry: onDeleteWorkspaceEntry,
    onLoadGitStatus: onLoadWorkspaceGitStatus,
    onLoadGitHistory: onLoadWorkspaceGitHistory,
    onLoadGitDiff: onLoadWorkspaceGitDiff,
  });
  const workspaceIDRef = useRef(projectWorkspace?.id);
  const loadFileRef = useRef(controller.loadFile);
  const loadTreeRef = useRef(controller.loadTree);
  const loadedWorkspaceIDRef = useRef<string | undefined>(undefined);
  workspaceIDRef.current = projectWorkspace?.id;
  loadFileRef.current = controller.loadFile;
  loadTreeRef.current = controller.loadTree;

  useEffect(() => {
    setMobileProjectFilesView("tree");
  }, [projectID, projectWorkspace?.id]);

  useEffect(() => {
    if (!projectFilesOpen) {
      loadedWorkspaceIDRef.current = undefined;
      return;
    }
    if (!projectWorkspace?.id || loadedWorkspaceIDRef.current === projectWorkspace.id) {
      return;
    }
    loadedWorkspaceIDRef.current = projectWorkspace.id;
    void controller.loadTree({ quiet: true });
  }, [projectFilesOpen, projectWorkspace?.id, controller.loadTree]);

  const closeMobileDrawers = useCallback(() => {
    setMobileNavOpen(false);
    setMobileAgentPanelOpen(false);
    setMobileMembersPanelOpen(false);
  }, [setMobileAgentPanelOpen, setMobileMembersPanelOpen, setMobileNavOpen]);

  const openProjectFiles = useCallback(() => {
    if (!projectWorkspace?.id) return;
    blurActiveElement();
    closeMobileDrawers();
    setMobileProjectFilesView("tree");
    setMobileEditorHeaderCollapsed(false);
    setProjectFilesOpen(true);
    loadedWorkspaceIDRef.current = projectWorkspace.id;
    void controller.loadTree({ quiet: true });
  }, [closeMobileDrawers, projectWorkspace?.id, controller.loadTree]);

  const openWorkspacePath = useCallback((target: WorkspacePathTarget) => {
    const workspaceID = workspaceIDRef.current;
    if (!workspaceID) return;
    blurActiveElement();
    closeMobileDrawers();
    setMobileProjectFilesView("editor");
    setProjectFilesOpen(true);
    loadedWorkspaceIDRef.current = workspaceID;
    void loadFileRef.current(target.path, {
      position: target.lineNumber
        ? { lineNumber: target.lineNumber, column: target.column ?? 1 }
        : undefined,
    });
    void loadTreeRef.current({ quiet: true });
  }, [closeMobileDrawers]);

  const toggleProjectFiles = useCallback(() => {
    if (projectFilesOpen) {
      blurActiveElement();
      setProjectFilesOpen(false);
      setProjectFileTreeCollapsed(false);
      setMobileProjectFilesView("tree");
      setMobileEditorHeaderCollapsed(false);
      return;
    }
    openProjectFiles();
  }, [openProjectFiles, projectFilesOpen]);

  const closeProjectFiles = useCallback(() => {
    setProjectFilesOpen(false);
    setMobileProjectFilesView("tree");
  }, []);

  const handleMobileProjectFilesBack = useCallback(() => {
    if (mobileProjectFilesView === "editor") {
      setMobileProjectFilesView("tree");
      setMobileEditorHeaderCollapsed(false);
      return;
    }
    setProjectFilesOpen(false);
    setProjectFileTreeCollapsed(false);
    setMobileProjectFilesView("tree");
    setMobileEditorHeaderCollapsed(false);
  }, [mobileProjectFilesView]);

  return {
    projectFilesOpen,
    projectFileTreeCollapsed,
    setProjectFileTreeCollapsed,
    mobileProjectFilesView,
    setMobileProjectFilesView,
    mobileEditorHeaderCollapsed,
    setMobileEditorHeaderCollapsed,
    projectFilesController: controller,
    openWorkspacePath,
    toggleProjectFiles,
    closeProjectFiles,
    handleMobileProjectFilesBack,
  };
}
