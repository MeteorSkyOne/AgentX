import { useEffect, useState } from "react";
import { Database, RefreshCw, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import type { WorkspaceFileBrowserProps } from "./workspace-browser/types";
import { useWorkspaceFileBrowser } from "./workspace-browser/useWorkspaceFileBrowser";
import { WorkspaceFileTree } from "./workspace-browser/WorkspaceFileTreePane";
import { WorkspaceFileTabBar } from "./workspace-browser/WorkspaceFileTabs";
import {
  WorkspaceFileDeleteDialog,
  WorkspaceFileEditor,
  WorkspaceFileToolbar,
} from "./workspace-browser/WorkspaceFileEditorPane";

export type {
  WorkspaceFileBrowserActions,
  WorkspaceFileBrowserController,
  WorkspaceFileBrowserProps,
  WorkspaceFileOpenOptions,
  WorkspaceFilePosition,
  WorkspaceFileTab,
  WorkspaceFileViewMode,
  WorkspacePaneView,
} from "./workspace-browser/types";
export { useWorkspaceFileBrowser } from "./workspace-browser/useWorkspaceFileBrowser";
export { WorkspaceFileTreePane, WorkspaceFileTree } from "./workspace-browser/WorkspaceFileTreePane";
export type { WorkspaceFileEditorProps } from "./workspace-browser/WorkspaceFileEditorPane";
export { WorkspaceFileEditorPane, WorkspaceGitDiffPane } from "./workspace-browser/WorkspaceFileEditorPane";

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
