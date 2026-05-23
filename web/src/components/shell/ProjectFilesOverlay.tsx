import type { Dispatch, SetStateAction } from "react";
import { ChevronLeft, ChevronRight, FolderOpen } from "lucide-react";
import type { ThemeMode } from "@/theme";
import { Button } from "@/components/ui/button";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import type { WorkspaceFileBrowserController } from "../WorkspaceFileBrowser";
import { WorkspaceFileEditorPane, WorkspaceGitDiffPane, WorkspaceFileTreePane } from "../WorkspaceFileBrowser";

export interface ProjectFilesOverlayProps {
  open: boolean;
  controller: WorkspaceFileBrowserController;
  theme: ThemeMode;
  treeCollapsed: boolean;
  setTreeCollapsed: Dispatch<SetStateAction<boolean>>;
  onClose: () => void;
  onChangeSelected: () => void;
}

export function ProjectFilesOverlay({
  open,
  controller: projectFilesController,
  theme,
  treeCollapsed: projectFileTreeCollapsed,
  setTreeCollapsed: setProjectFileTreeCollapsed,
  onClose: toggleProjectFiles,
  onChangeSelected,
}: ProjectFilesOverlayProps) {
  if (!open) return null;

  return (

        <div
          className="absolute inset-0 z-30 min-h-0 min-w-0 bg-background shadow-2xl"
          data-testid="project-files-overlay"
        >
          {projectFileTreeCollapsed ? (
            <div className="relative h-full min-h-0 min-w-0">
              {projectFilesController.workspacePaneView === "changes" ? (
                <WorkspaceGitDiffPane
                  controller={projectFilesController}
                  theme={theme}
                  contentAriaLabel="Project git diff preview"
                  viewerClassName="md:mx-6"
                  toolbarEnd={
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 shrink-0 bg-accent"
                      title="Close project files"
                      aria-label="Close project files"
                      aria-pressed="true"
                      onClick={toggleProjectFiles}
                    >
                      <FolderOpen className="h-4 w-4" />
                    </Button>
                  }
                />
              ) : (
                <WorkspaceFileEditorPane
                  controller={projectFilesController}
                  theme={theme}
                  contentAriaLabel="Project file editor"
                  editorClassName="md:mx-6"
                  toolbarEnd={
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 shrink-0 bg-accent"
                      title="Close project files"
                      aria-label="Close project files"
                      aria-pressed="true"
                      onClick={toggleProjectFiles}
                    >
                      <FolderOpen className="h-4 w-4" />
                    </Button>
                  }
                />
              )}
              <div className="pointer-events-none absolute left-0 top-1/2 z-40 -translate-y-1/2">
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="pointer-events-auto h-10 w-5 cursor-pointer rounded-l-none rounded-r-full border border-l-0 border-border bg-sidebar shadow-md hover:bg-accent"
                  title="Show project file tree"
                  aria-label="Show project file tree"
                  aria-expanded="false"
                  onClick={() => setProjectFileTreeCollapsed(false)}
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ) : (
            <ResizablePanelGroup direction="horizontal" className="h-full">
              <>
                <ResizablePanel defaultSize={24} minSize={18} maxSize={36}>
                  <WorkspaceFileTreePane
                    controller={projectFilesController}
                    title="Project files"
                    ariaLabel="Project files"
                    onChangeSelected={onChangeSelected}
                    toolbarEnd={
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        className="h-8 w-8"
                        title="Hide project file tree"
                        aria-label="Hide project file tree"
                        aria-expanded="true"
                        onClick={() => setProjectFileTreeCollapsed(true)}
                      >
                        <ChevronLeft className="h-4 w-4" />
                      </Button>
                    }
                  />
                </ResizablePanel>
                <ResizableHandle withHandle />
              </>
              <ResizablePanel defaultSize={76} minSize={48}>
                {projectFilesController.workspacePaneView === "changes" ? (
                  <WorkspaceGitDiffPane
                    controller={projectFilesController}
                    theme={theme}
                    contentAriaLabel="Project git diff preview"
                    toolbarEnd={
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 shrink-0 bg-accent"
                        title="Close project files"
                        aria-label="Close project files"
                        aria-pressed="true"
                        onClick={toggleProjectFiles}
                      >
                        <FolderOpen className="h-4 w-4" />
                      </Button>
                    }
                  />
                ) : (
                  <WorkspaceFileEditorPane
                    controller={projectFilesController}
                    theme={theme}
                    contentAriaLabel="Project file editor"
                    toolbarEnd={
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 shrink-0 bg-accent"
                        title="Close project files"
                        aria-label="Close project files"
                        aria-pressed="true"
                        onClick={toggleProjectFiles}
                      >
                        <FolderOpen className="h-4 w-4" />
                      </Button>
                    }
                  />
                )}
              </ResizablePanel>
            </ResizablePanelGroup>
          )}
        </div>  );
}
