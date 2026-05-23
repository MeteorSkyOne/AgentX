import type { Dispatch, SetStateAction } from "react";
import {
  Activity,
  ArrowLeft,
  CalendarClock,
  ChevronDown,
  ChevronUp,
  FolderOpen,
  LogOut,
  Menu,
  Moon,
  Pencil,
  Plus,
  Settings,
  SquareTerminal,
  Sun,
  Trash2,
  UserRound,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import type { Agent, Channel, ConversationAgentContext, Thread } from "../../api/types";
import type { ThemeMode } from "../../theme";
import type { WorkspaceFileBrowserController } from "../WorkspaceFileBrowser";
import {
  WorkspaceFileEditorPane,
  WorkspaceFileTreePane,
  WorkspaceGitDiffPane,
} from "../WorkspaceFileBrowser";
import { ChannelList } from "../ChannelList";
import { AgentDetailsPanel } from "./AgentDetailsPanel";
import { AgentsSidebar } from "./AgentsSidebar";
import { ConversationPanel } from "./ConversationPanel";
import { MembersPanel } from "./MembersPanel";
import { MetricsPanel } from "./MetricsPanel";
import { TasksPanel } from "./TasksPanel";
import { TerminalDockBoundary } from "./LazyTerminalDock";
import type { ShellProps } from "./types";
import { getProjectAvatar, initials } from "./utils";

interface MobileShellProps {
  projectFilesOpen: boolean;
  mobileProjectFilesView: "tree" | "editor";
  handleMobileProjectFilesBack: () => void;
  mobileTerminalOpen: boolean;
  setTerminalOpen: Dispatch<SetStateAction<boolean>>;
  selectedChannel?: Channel;
  activeThread?: Thread;
  onSelectChannel: ShellProps["onSelectChannel"];
  headerTitle: string;
  projectWorkspace: ShellProps["projectWorkspace"];
  headerSubtitle: string;
  showMobileEditorHeaderControls: boolean;
  toggleProjectFiles: () => void;
  mobileEditorHeaderCollapsed: boolean;
  setMobileEditorHeaderCollapsed: Dispatch<SetStateAction<boolean>>;
  showMobileProjectFilesButton: boolean;
  mainView: "chat" | "metrics" | "tasks";
  project: ShellProps["project"];
  openTasks: () => void;
  terminalAllowed: boolean;
  terminalOpen: boolean;
  setMobileMembersPanelOpen: Dispatch<SetStateAction<boolean>>;
  setMobileAgentPanelOpen: Dispatch<SetStateAction<boolean>>;
  activityLabel: string;
  threadActionPending: boolean;
  deleteActiveThread: () => Promise<void>;
  setThreadTitleDraft: Dispatch<SetStateAction<string>>;
  setThreadActionError: Dispatch<SetStateAction<string | null>>;
  setThreadEditOpen: Dispatch<SetStateAction<boolean>>;
  theme: ThemeMode;
  projectFilesController: WorkspaceFileBrowserController;
  setMobileProjectFilesView: Dispatch<SetStateAction<"tree" | "editor">>;
  channels: ShellProps["channels"];
  threads: ShellProps["threads"];
  activeConversation: ShellProps["activeConversation"];
  activeAgents: Agent[];
  messages: ShellProps["messages"];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: ShellProps["streaming"];
  pendingQuestion: ShellProps["pendingQuestion"];
  queuedPrompts: ShellProps["queuedPrompts"];
  boundAgents: ConversationAgentContext[];
  preferences: ShellProps["preferences"];
  composerConversation: ShellProps["activeConversation"] extends infer _ ? import("./types").ComposerConversation | undefined : never;
  onSelectThread: ShellProps["onSelectThread"];
  onCreateThread: ShellProps["onCreateThread"];
  onUpdateThread: ShellProps["onUpdateThread"];
  onDeleteThread: ShellProps["onDeleteThread"];
  onUpdateMessage: ShellProps["onUpdateMessage"];
  onDeleteMessage: ShellProps["onDeleteMessage"];
  onLoadOlderMessages: ShellProps["onLoadOlderMessages"];
  onRespondToQuestion: ShellProps["onRespondToQuestion"];
  onSteerQueuedPrompt: ShellProps["onSteerQueuedPrompt"];
  onDeleteQueuedPrompt: ShellProps["onDeleteQueuedPrompt"];
  onMessageSent: ShellProps["onMessageSent"];
  openWorkspacePath: (target: WorkspacePathTarget) => void;
  mobileNavOpen: boolean;
  setMobileNavOpen: Dispatch<SetStateAction<boolean>>;
  projects: ShellProps["projects"];
  selectMobileProject: (projectID: string) => void;
  openCreateProject: () => void;
  openProjectSettings: () => void;
  openMetrics: () => void;
  onUpdateChannel: ShellProps["onUpdateChannel"];
  onDeleteChannel: ShellProps["onDeleteChannel"];
  setChannelDraftOpen: Dispatch<SetStateAction<boolean>>;
  contextLoading: boolean;
  setFocusedAgentID: Dispatch<SetStateAction<string>>;
  setAgentDraftOpen: Dispatch<SetStateAction<boolean>>;
  user: ShellProps["user"];
  organization: ShellProps["organization"];
  openAccountSettings: () => void;
  onToggleTheme: ShellProps["onToggleTheme"];
  onLogout: ShellProps["onLogout"];
  mobileMembersPanelOpen: boolean;
  onSaveChannelAgents: ShellProps["onSaveChannelAgents"];
  mobileAgentPanelOpen: boolean;
  selectedAgent?: Agent;
  onUpdateAgent: ShellProps["onUpdateAgent"];
  onDeleteAgent: ShellProps["onDeleteAgent"];
  onLoadWorkspaceTree: ShellProps["onLoadWorkspaceTree"];
  onSearchWorkspace: ShellProps["onSearchWorkspace"];
  onReadWorkspaceFile: ShellProps["onReadWorkspaceFile"];
  onFetchWorkspaceFileBlob: ShellProps["onFetchWorkspaceFileBlob"];
  onWriteWorkspaceFile: ShellProps["onWriteWorkspaceFile"];
  onDeleteWorkspaceFile: ShellProps["onDeleteWorkspaceFile"];
  onCreateWorkspaceEntry: ShellProps["onCreateWorkspaceEntry"];
  onMoveWorkspaceEntry: ShellProps["onMoveWorkspaceEntry"];
  onDeleteWorkspaceEntry: ShellProps["onDeleteWorkspaceEntry"];
  selectMobileChannel: (channel: Channel) => void;
}

export function MobileShell({
  projectFilesOpen,
  mobileProjectFilesView,
  handleMobileProjectFilesBack,
  mobileTerminalOpen,
  setTerminalOpen,
  selectedChannel,
  activeThread,
  onSelectChannel,
  headerTitle,
  projectWorkspace,
  headerSubtitle,
  showMobileEditorHeaderControls,
  toggleProjectFiles,
  mobileEditorHeaderCollapsed,
  setMobileEditorHeaderCollapsed,
  showMobileProjectFilesButton,
  mainView,
  project,
  openTasks,
  terminalAllowed,
  terminalOpen,
  setMobileMembersPanelOpen,
  setMobileAgentPanelOpen,
  activityLabel,
  threadActionPending,
  deleteActiveThread,
  setThreadTitleDraft,
  setThreadActionError,
  setThreadEditOpen,
  theme,
  projectFilesController,
  setMobileProjectFilesView,
  channels,
  threads,
  activeConversation,
  activeAgents,
  messages,
  messagesLoading,
  olderMessagesLoading,
  hasOlderMessages,
  streaming,
  pendingQuestion,
  queuedPrompts,
  boundAgents,
  preferences,
  composerConversation,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onRespondToQuestion,
  onSteerQueuedPrompt,
  onDeleteQueuedPrompt,
  onMessageSent,
  openWorkspacePath,
  mobileNavOpen,
  setMobileNavOpen,
  projects,
  selectMobileProject,
  openCreateProject,
  openProjectSettings,
  openMetrics,
  onUpdateChannel,
  onDeleteChannel,
  setChannelDraftOpen,
  contextLoading,
  setFocusedAgentID,
  setAgentDraftOpen,
  user,
  organization,
  openAccountSettings,
  onToggleTheme,
  onLogout,
  mobileMembersPanelOpen,
  onSaveChannelAgents,
  mobileAgentPanelOpen,
  selectedAgent,
  onUpdateAgent,
  onDeleteAgent,
  onLoadWorkspaceTree,
  onSearchWorkspace,
  onReadWorkspaceFile,
  onFetchWorkspaceFileBlob,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onCreateWorkspaceEntry,
  onMoveWorkspaceEntry,
  onDeleteWorkspaceEntry,
  selectMobileChannel,
}: MobileShellProps) {
  return (
      <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="mobile-shell">
        <div className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-2">
          {projectFilesOpen ? (
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11"
              title={mobileProjectFilesView === "editor" ? "Back to files" : "Back to chat"}
              aria-label={mobileProjectFilesView === "editor" ? "Back to files" : "Back to chat"}
              onClick={handleMobileProjectFilesBack}
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
          ) : null}
          {mobileTerminalOpen ? (
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11"
              title="Back to chat"
              aria-label="Back to chat"
              onClick={() => setTerminalOpen(false)}
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
          ) : null}
          {!projectFilesOpen && !mobileTerminalOpen ? (
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11"
              title="Navigation"
              aria-label="Navigation"
              onClick={() => setMobileNavOpen(true)}
            >
              <Menu className="h-5 w-5" />
            </Button>
          ) : null}
          {!projectFilesOpen && !mobileTerminalOpen && selectedChannel?.type === "thread" && activeThread ? (
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11"
              title="Back to posts"
              aria-label="Back to posts"
              onClick={() => onSelectChannel(selectedChannel)}
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
          ) : null}
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-sm font-semibold">
              {projectFilesOpen ? "Project files" : mobileTerminalOpen ? "Terminal" : headerTitle}
            </h1>
            <p className="truncate text-xs text-muted-foreground">
              {projectFilesOpen || mobileTerminalOpen ? projectWorkspace?.path ?? "No workspace" : headerSubtitle}
            </p>
          </div>
          {showMobileEditorHeaderControls ? (
            <>
              <Button
                variant="ghost"
                size="icon"
                className="h-11 w-11 bg-accent"
                title="Close project files"
                aria-label="Close project files"
                aria-pressed="true"
                onClick={toggleProjectFiles}
              >
                <FolderOpen className="h-5 w-5" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-11 w-11"
                title={mobileEditorHeaderCollapsed ? "Show file path bar" : "Hide file path bar"}
                aria-label={mobileEditorHeaderCollapsed ? "Show file path bar" : "Hide file path bar"}
                aria-expanded={!mobileEditorHeaderCollapsed}
                onClick={() => setMobileEditorHeaderCollapsed((collapsed) => !collapsed)}
              >
                {mobileEditorHeaderCollapsed ? (
                  <ChevronDown className="h-5 w-5" />
                ) : (
                  <ChevronUp className="h-5 w-5" />
                )}
              </Button>
            </>
          ) : showMobileProjectFilesButton && !mobileTerminalOpen ? (
            <>
              <Button
                variant="ghost"
                size="icon"
                className={cn("h-11 w-11", mainView === "tasks" && "bg-accent")}
                title="Tasks"
                aria-label="Tasks"
                aria-pressed={mainView === "tasks"}
                disabled={!project}
                onClick={openTasks}
              >
                <CalendarClock className="h-5 w-5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className={cn("h-11 w-11", projectFilesOpen && "bg-accent")}
                title="Project files"
                aria-label="Project files"
                aria-pressed={projectFilesOpen}
                disabled={!projectWorkspace?.id}
                onClick={toggleProjectFiles}
              >
                <FolderOpen className="h-5 w-5" />
              </Button>
              {terminalAllowed ? (
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-11 w-11", terminalOpen && "bg-accent")}
                  title="Terminal"
                  aria-label="Terminal"
                  aria-pressed={terminalOpen}
                  disabled={!projectWorkspace?.id}
                  onClick={() => setTerminalOpen(true)}
                >
                  <SquareTerminal className="h-5 w-5" />
                </Button>
              ) : null}
            </>
          ) : null}
          {!projectFilesOpen && !mobileTerminalOpen ? (
            <>
              <Button
                variant="ghost"
                size="icon"
                className="h-11 w-11"
                title="Members"
                aria-label="Members"
                onClick={() => setMobileMembersPanelOpen(true)}
              >
                <UserRound className="h-5 w-5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-11 w-11"
                title="Agent settings"
                aria-label="Agent settings"
                onClick={() => setMobileAgentPanelOpen(true)}
              >
                <Settings className="h-5 w-5" />
              </Button>
            </>
          ) : null}
        </div>

        {!projectFilesOpen && !mobileTerminalOpen && activeThread && (
          <div className="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
            {activeConversation && (
              <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                <Activity className="h-3.5 w-3.5 shrink-0" />
                {activityLabel}
              </span>
            )}
            <div className="ml-auto flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                className="h-9 w-9"
                title="Edit post"
                aria-label="Edit post"
                onClick={() => {
                  setThreadTitleDraft(activeThread.title);
                  setThreadActionError(null);
                  setThreadEditOpen(true);
                }}
              >
                <Pencil className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-9 w-9 text-muted-foreground hover:text-destructive"
                title="Delete post"
                aria-label="Delete post"
                disabled={threadActionPending}
                onClick={deleteActiveThread}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}

        <div className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          {mobileTerminalOpen ? (
            <TerminalDockBoundary
                              workspace={projectWorkspace}
                theme={theme}
                className="min-h-0 flex-1"
                onClose={() => setTerminalOpen(false)}
              />
            
          ) : mainView === "metrics" ? (
            <MetricsPanel
              project={project}
              selectedChannel={selectedChannel}
              activeConversation={activeConversation}
            />
          ) : mainView === "tasks" ? (
            <TasksPanel
              project={project}
              projectWorkspace={projectWorkspace}
              channels={channels}
              threads={threads}
              activeConversation={activeConversation}
              agents={activeAgents}
            />
          ) : (
            <ConversationPanel
              selectedChannel={selectedChannel}
              activeThread={activeThread}
              threads={threads}
              messages={messages}
              messagesLoading={messagesLoading}
              olderMessagesLoading={olderMessagesLoading}
              hasOlderMessages={hasOlderMessages}
              streaming={streaming}
              pendingQuestion={pendingQuestion}
              queuedPrompts={queuedPrompts}
              boundAgents={boundAgents}
              preferences={preferences}
              theme={theme}
              composerConversation={composerConversation}
              onSelectThread={onSelectThread}
              onCreateThread={onCreateThread}
              onUpdateThread={onUpdateThread}
              onDeleteThread={onDeleteThread}
              onUpdateMessage={onUpdateMessage}
              onDeleteMessage={onDeleteMessage}
              onLoadOlderMessages={onLoadOlderMessages}
              onRespondToQuestion={onRespondToQuestion}
              onSteerQueuedPrompt={onSteerQueuedPrompt}
              onDeleteQueuedPrompt={onDeleteQueuedPrompt}
              onMessageSent={onMessageSent}
              workspacePath={projectWorkspace?.path}
              onOpenWorkspacePath={openWorkspacePath}
            />
          )}
          {projectFilesOpen && (
            <div
              className="absolute inset-0 z-20 flex min-h-0 flex-col bg-background"
              data-testid="project-files-overlay"
            >
              {mobileProjectFilesView === "tree" ? (
                <WorkspaceFileTreePane
                  controller={projectFilesController}
                  title="Project files"
                  ariaLabel="Project files"
                  className="min-h-0 flex-1"
                  onFileSelected={() => {
                    setMobileProjectFilesView("editor");
                    setMobileEditorHeaderCollapsed(false);
                  }}
                  onChangeSelected={() => {
                    setMobileProjectFilesView("editor");
                    setMobileEditorHeaderCollapsed(false);
                  }}
                />
              ) : (
                projectFilesController.workspacePaneView === "changes" ? (
                  <WorkspaceGitDiffPane
                    controller={projectFilesController}
                    theme={theme}
                    contentAriaLabel="Project git diff preview"
                    className="min-h-0 flex-1"
                  />
                ) : (
                  <WorkspaceFileEditorPane
                    controller={projectFilesController}
                    theme={theme}
                    contentAriaLabel="Project file editor"
                    className="min-h-0 flex-1"
                    headerCollapsed={mobileEditorHeaderCollapsed}
                    onHeaderCollapsedChange={setMobileEditorHeaderCollapsed}
                    headerControlsPlacement="external"
                  />
                )
              )}
            </div>
          )}
        </div>

        <Dialog open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
          <DialogContent
            showCloseButton={false}
            className="left-0 top-0 min-w-0 !h-svh !w-[100svw] !max-w-[100svw] !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-l-0 p-0 sm:!w-[24rem] sm:!max-w-sm"
          >
            <div className="flex h-full min-h-0 min-w-0 flex-col overflow-x-hidden bg-sidebar">
              <div
                className="flex h-14 min-w-0 shrink-0 items-center justify-between gap-3 border-b border-border px-4"
                data-testid="mobile-nav-header"
              >
                <DialogHeader className="min-w-0 flex-1 gap-0 text-left">
                  <DialogTitle className="truncate">Navigation</DialogTitle>
                  <DialogDescription className="truncate">{project?.name ?? "No project"}</DialogDescription>
                </DialogHeader>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-10 w-10"
                    title="Project settings"
                    aria-label="Project settings"
                    disabled={!project}
                    onClick={() => {
                      setMobileNavOpen(false);
                      openProjectSettings();
                    }}
                  >
                    <Settings className="h-5 w-5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-10 w-10"
                    title="Close navigation"
                    aria-label="Close navigation"
                    onClick={() => setMobileNavOpen(false)}
                  >
                    <X className="h-5 w-5" />
                  </Button>
                </div>
              </div>

              <ScrollArea
                className="min-h-0 min-w-0 flex-1"
                viewportClassName="max-w-full overflow-x-hidden"
                data-testid="mobile-nav-scroll"
              >
                <div className="min-w-0 max-w-full space-y-5 px-3 py-4">
                  <section aria-label="Projects" className="min-w-0 max-w-full space-y-1">
                    <div className="px-1 text-xs font-semibold uppercase text-muted-foreground">
                      Projects
                    </div>
                    {projects.map((item) => {
                      const avatar = getProjectAvatar(item.id);
                      const isSelected = item.id === project?.id;
                      return (
                        <button
                          key={item.id}
                          className={cn(
                            "flex min-h-11 min-w-0 max-w-full w-full items-center gap-3 overflow-hidden rounded-md px-2 text-left text-sm transition-colors",
                            isSelected
                              ? "bg-accent text-accent-foreground"
                              : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                          )}
                          aria-label={item.name}
                          onClick={() => selectMobileProject(item.id)}
                        >
                          <span
                            className={cn(
                              "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-xs font-semibold",
                              avatar?.emoji
                                ? cn("text-white", avatar.color || "bg-primary")
                                : "bg-secondary text-secondary-foreground"
                            )}
                          >
                            {avatar?.emoji ? avatar.emoji : initials(item.name)}
                          </span>
                          <span className="block min-w-0 max-w-[calc(100svw-8rem)] flex-1 truncate">{item.name}</span>
                        </button>
                      );
                    })}
                    <button
                      className="flex min-h-11 min-w-0 max-w-full w-full items-center gap-3 overflow-hidden rounded-md px-2 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                      title="Create project"
                      aria-label="Create project"
                      onClick={() => {
                        setMobileNavOpen(false);
                        openCreateProject();
                      }}
                    >
                      <Plus className="h-4 w-4 shrink-0" />
                      <span className="min-w-0 truncate">Create project</span>
                    </button>
                  </section>

                  <ChannelList
                    channels={channels}
                    selectedChannelID={selectedChannel?.id}
                    metricsActive={mainView === "metrics"}
                    onSelect={selectMobileChannel}
                    onOpenMetrics={openMetrics}
                    onCreate={() => {
                      setMobileNavOpen(false);
                      setChannelDraftOpen(true);
                    }}
                    onUpdate={onUpdateChannel}
                    onDelete={onDeleteChannel}
                  />

                  <button
                    className={cn(
                      "mt-2 flex min-h-10 w-full items-center gap-2 rounded-md px-2 text-left text-sm transition-colors",
                      mainView === "tasks"
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                    )}
                    disabled={!project}
                    onClick={() => {
                      setMobileNavOpen(false);
                      openTasks();
                    }}
                  >
                    <CalendarClock className="h-4 w-4 shrink-0" />
                    <span className="min-w-0 truncate">Tasks</span>
                  </button>

                  <AgentsSidebar
                    agents={activeAgents}
                    boundAgents={boundAgents}
                    contextLoading={contextLoading}
                    onOpenPanel={(agentID) => {
                      if (agentID) setFocusedAgentID(agentID);
                      setMobileNavOpen(false);
                      setMobileAgentPanelOpen(true);
                    }}
                    onCreateAgent={() => {
                      setMobileNavOpen(false);
                      setAgentDraftOpen(true);
                    }}
                  />
                </div>
              </ScrollArea>

              <div
                className="min-w-0 shrink-0 border-t border-border px-3 py-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))]"
                data-testid="mobile-nav-footer"
              >
                <div className="mb-2 flex min-w-0 items-center gap-3">
                  <Avatar className="h-9 w-9">
                    <AvatarFallback className="bg-primary text-xs text-primary-foreground">
                      {initials(user.display_name)}
                    </AvatarFallback>
                  </Avatar>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{user.display_name}</p>
                    <p className="truncate text-xs text-muted-foreground">{organization?.name ?? "online"}</p>
                  </div>
                </div>
                <div className="grid min-w-0 grid-cols-3 gap-2">
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-10 min-w-0 w-full"
                    title="User settings"
                    aria-label="User settings"
                    onClick={() => {
                      setMobileNavOpen(false);
                      openAccountSettings();
                    }}
                  >
                    <Settings className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-10 min-w-0 w-full"
                    title={theme === "dark" ? "Light mode" : "Dark mode"}
                    aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
                    onClick={onToggleTheme}
                  >
                    {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    className="h-10 min-w-0 w-full"
                    title="Log out"
                    aria-label="Log out"
                    onClick={onLogout}
                  >
                    <LogOut className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
          </DialogContent>
        </Dialog>

        <Dialog open={mobileMembersPanelOpen} onOpenChange={setMobileMembersPanelOpen}>
          <DialogContent
            showCloseButton={false}
            className="left-auto right-0 top-0 !h-svh w-[92vw] max-w-sm !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-y-0 border-r-0 p-0 sm:max-w-sm"
          >
            <DialogTitle className="sr-only">Members</DialogTitle>
            <DialogDescription className="sr-only">Manage channel members.</DialogDescription>
            <MembersPanel
              agents={activeAgents}
              boundAgents={boundAgents}
              projectWorkspace={projectWorkspace}
              selectedChannel={selectedChannel}
              onSaveChannelAgents={onSaveChannelAgents}
              onClose={() => setMobileMembersPanelOpen(false)}
            />
          </DialogContent>
        </Dialog>

        <Dialog open={mobileAgentPanelOpen} onOpenChange={setMobileAgentPanelOpen}>
          <DialogContent
            showCloseButton={false}
            data-testid="mobile-agent-settings-dialog"
            className="!left-0 !right-auto !top-0 min-w-0 !h-svh !w-[100svw] !max-w-[100svw] !translate-x-0 !translate-y-0 gap-0 overflow-hidden rounded-none border-0 p-0"
          >
            <DialogTitle className="sr-only">Agent settings</DialogTitle>
            <DialogDescription className="sr-only">Manage agent settings and workspace files.</DialogDescription>
            <AgentDetailsPanel
              selectedChannel={selectedChannel}
              projectWorkspace={projectWorkspace}
              agents={activeAgents}
              boundAgents={boundAgents}
              selectedAgent={selectedAgent}
              onUpdateAgent={onUpdateAgent}
              onDeleteAgent={onDeleteAgent}
              onLoadWorkspaceTree={onLoadWorkspaceTree}
              onSearchWorkspace={onSearchWorkspace}
              onReadWorkspaceFile={onReadWorkspaceFile}
              onFetchWorkspaceFileBlob={onFetchWorkspaceFileBlob}
              onWriteWorkspaceFile={onWriteWorkspaceFile}
              onDeleteWorkspaceFile={onDeleteWorkspaceFile}
              onCreateWorkspaceEntry={onCreateWorkspaceEntry}
              onMoveWorkspaceEntry={onMoveWorkspaceEntry}
              onDeleteWorkspaceEntry={onDeleteWorkspaceEntry}
              onCreateAgentModal={() => setAgentDraftOpen(true)}
              onClose={() => setMobileAgentPanelOpen(false)}
              theme={theme}
            />
          </DialogContent>
        </Dialog>
      </div>
  );
}
