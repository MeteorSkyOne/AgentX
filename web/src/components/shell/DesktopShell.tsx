import type { Dispatch, PointerEvent as ReactPointerEvent, RefObject, SetStateAction } from "react";
import {
  Activity,
  ArrowLeft,
  BarChart3,
  Bot,
  CalendarClock,
  ChevronDown,
  FolderOpen,
  Hash,
  LogOut,
  Moon,
  Pencil,
  Plus,
  Rows3,
  Settings,
  SquareTerminal,
  Sun,
  Trash2,
  UserRound,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { Agent, Channel, ConversationAgentContext, Thread } from "../../api/types";
import type { ThemeMode } from "../../theme";
import type { WorkspaceFileBrowserController } from "../WorkspaceFileBrowser";
import { ChannelList } from "../ChannelList";
import { AgentDetailsPanel } from "./AgentDetailsPanel";
import { AgentsSidebar } from "./AgentsSidebar";
import { ConversationPanel } from "./ConversationPanel";
import { MembersPanel } from "./MembersPanel";
import { MetricsPanel } from "./MetricsPanel";
import { ProjectFilesOverlay } from "./ProjectFilesOverlay";
import { TasksPanel } from "./TasksPanel";
import { TerminalDockBoundary } from "./LazyTerminalDock";
import type { ComposerConversation, ShellProps } from "./types";
import { agentToneColor, getProjectAvatar, initials } from "./utils";

interface DesktopShellProps {
  projectFilesOpen: boolean;
  projectDraftOpen: boolean;
  openCreateProject: () => void;
  projects: ShellProps["projects"];
  project: ShellProps["project"];
  theme: ThemeMode;
  onToggleTheme: ShellProps["onToggleTheme"];
  onLogout: ShellProps["onLogout"];
  onSelectProject: ShellProps["onSelectProject"];
  openProjectSettings: () => void;
  mainView: "chat" | "metrics" | "tasks";
  channels: ShellProps["channels"];
  selectedChannel?: Channel;
  selectSidebarChannel: (channel: Channel) => void;
  openMetrics: () => void;
  onUpdateChannel: ShellProps["onUpdateChannel"];
  onDeleteChannel: ShellProps["onDeleteChannel"];
  setChannelDraftOpen: Dispatch<SetStateAction<boolean>>;
  openTasks: () => void;
  activeAgents: Agent[];
  boundAgents: ConversationAgentContext[];
  contextLoading: boolean;
  setFocusedAgentID: Dispatch<SetStateAction<string>>;
  setMembersPanelOpen: Dispatch<SetStateAction<boolean>>;
  setAgentPanelOpen: Dispatch<SetStateAction<boolean>>;
  setAgentDraftOpen: Dispatch<SetStateAction<boolean>>;
  onSaveChannelAgents: ShellProps["onSaveChannelAgents"];
  user: ShellProps["user"];
  organization: ShellProps["organization"];
  openAccountSettings: () => void;
  agentPanelOpen: boolean;
  membersPanelOpen: boolean;
  activeThread?: Thread;
  title: string;
  subtitle: string;
  onSelectChannel: ShellProps["onSelectChannel"];
  threadActionPending: boolean;
  deleteActiveThread: () => Promise<void>;
  setThreadTitleDraft: Dispatch<SetStateAction<string>>;
  setThreadActionError: Dispatch<SetStateAction<string | null>>;
  setThreadEditOpen: Dispatch<SetStateAction<boolean>>;
  activeConversation: ShellProps["activeConversation"];
  activityLabel: string;
  projectWorkspace: ShellProps["projectWorkspace"];
  toggleProjectFiles: () => void;
  terminalAllowed: boolean;
  terminalOpen: boolean;
  setTerminalOpen: Dispatch<SetStateAction<boolean>>;
  terminalResizeContainerRef: RefObject<HTMLDivElement | null>;
  startTerminalResize: (event: ReactPointerEvent<HTMLDivElement>) => void;
  terminalHeightPct: number;
  threads: ShellProps["threads"];
  messages: ShellProps["messages"];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: ShellProps["streaming"];
  pendingQuestion: ShellProps["pendingQuestion"];
  queuedPrompts: ShellProps["queuedPrompts"];
  preferences: ShellProps["preferences"];
  composerConversation?: ComposerConversation;
  onSelectThread: ShellProps["onSelectThread"];
  onCreateThread: ShellProps["onCreateThread"];
  onUpdateThread: ShellProps["onUpdateThread"];
  onDeleteThread: ShellProps["onDeleteThread"];
  onUpdateMessage: ShellProps["onUpdateMessage"];
  onDeleteMessage: ShellProps["onDeleteMessage"];
  onRetryMessage?: ShellProps["onRetryMessage"];
  onLoadOlderMessages: ShellProps["onLoadOlderMessages"];
  onRespondToQuestion: ShellProps["onRespondToQuestion"];
  onSteerQueuedPrompt: ShellProps["onSteerQueuedPrompt"];
  onDeleteQueuedPrompt: ShellProps["onDeleteQueuedPrompt"];
  onMessageSent: ShellProps["onMessageSent"];
  openWorkspacePath: (target: WorkspacePathTarget) => void;
  selectedAgent?: Agent;
  onUpdateAgent: ShellProps["onUpdateAgent"];
  onDeleteAgent: ShellProps["onDeleteAgent"];
  toolUpdates: ShellProps["toolUpdates"];
  toolUpdatesLoading: boolean;
  onCheckToolUpdates: ShellProps["onCheckToolUpdates"];
  onRunToolUpdate: ShellProps["onRunToolUpdate"];
  onLoadWorkspaceTree: ShellProps["onLoadWorkspaceTree"];
  onSearchWorkspace: ShellProps["onSearchWorkspace"];
  onReadWorkspaceFile: ShellProps["onReadWorkspaceFile"];
  onFetchWorkspaceFileBlob: ShellProps["onFetchWorkspaceFileBlob"];
  onWriteWorkspaceFile: ShellProps["onWriteWorkspaceFile"];
  onDeleteWorkspaceFile: ShellProps["onDeleteWorkspaceFile"];
  onCreateWorkspaceEntry: ShellProps["onCreateWorkspaceEntry"];
  onMoveWorkspaceEntry: ShellProps["onMoveWorkspaceEntry"];
  onDeleteWorkspaceEntry: ShellProps["onDeleteWorkspaceEntry"];
  projectFilesController: WorkspaceFileBrowserController;
  projectFileTreeCollapsed: boolean;
  setProjectFileTreeCollapsed: Dispatch<SetStateAction<boolean>>;
  setMobileProjectFilesView: Dispatch<SetStateAction<"tree" | "editor">>;
}

export function DesktopShell({
  projectFilesOpen,
  projectDraftOpen,
  openCreateProject,
  projects,
  project,
  theme,
  onToggleTheme,
  onLogout,
  onSelectProject,
  openProjectSettings,
  mainView,
  channels,
  selectedChannel,
  selectSidebarChannel,
  openMetrics,
  onUpdateChannel,
  onDeleteChannel,
  setChannelDraftOpen,
  openTasks,
  activeAgents,
  boundAgents,
  contextLoading,
  setFocusedAgentID,
  setMembersPanelOpen,
  setAgentPanelOpen,
  setAgentDraftOpen,
  onSaveChannelAgents,
  user,
  organization,
  openAccountSettings,
  agentPanelOpen,
  membersPanelOpen,
  activeThread,
  title,
  subtitle,
  onSelectChannel,
  threadActionPending,
  deleteActiveThread,
  setThreadTitleDraft,
  setThreadActionError,
  setThreadEditOpen,
  activeConversation,
  activityLabel,
  projectWorkspace,
  toggleProjectFiles,
  terminalAllowed,
  terminalOpen,
  setTerminalOpen,
  terminalResizeContainerRef,
  startTerminalResize,
  terminalHeightPct,
  threads,
  messages,
  messagesLoading,
  olderMessagesLoading,
  hasOlderMessages,
  streaming,
  pendingQuestion,
  queuedPrompts,
  preferences,
  composerConversation,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread,
  onUpdateMessage,
  onDeleteMessage,
  onRetryMessage,
  onLoadOlderMessages,
  onRespondToQuestion,
  onSteerQueuedPrompt,
  onDeleteQueuedPrompt,
  onMessageSent,
  openWorkspacePath,
  selectedAgent,
  onUpdateAgent,
  onDeleteAgent,
  toolUpdates,
  toolUpdatesLoading,
  onCheckToolUpdates,
  onRunToolUpdate,
  onLoadWorkspaceTree,
  onSearchWorkspace,
  onReadWorkspaceFile,
  onFetchWorkspaceFileBlob,
  onWriteWorkspaceFile,
  onDeleteWorkspaceFile,
  onCreateWorkspaceEntry,
  onMoveWorkspaceEntry,
  onDeleteWorkspaceEntry,
  projectFilesController,
  projectFileTreeCollapsed,
  setProjectFileTreeCollapsed,
  setMobileProjectFilesView,
}: DesktopShellProps) {
  return (
      <div className="flex h-full min-h-0 min-w-0 flex-1" data-testid="desktop-shell">
      {/* Project Rail */}
      <TooltipProvider delayDuration={0}>
        <div className="flex h-full w-[72px] flex-col items-center gap-2 border-r border-sidebar-border/70 bg-sidebar py-3">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-primary text-primary-foreground font-bold text-lg"
              >
                AX
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">AgentX</TooltipContent>
          </Tooltip>

          <div className="mx-auto h-0.5 w-8 shrink-0 rounded-full bg-border" />

          <ScrollArea className="min-h-0 w-full flex-1">
            <div className="flex flex-col items-center gap-2">
              {projects.map((item) => {
                const avatar = getProjectAvatar(item.id);
                const isSelected = item.id === project?.id;
                return (
                  <Tooltip key={item.id}>
                    <TooltipTrigger asChild>
                      <button
                        className={cn(
                          "relative flex h-12 w-12 items-center justify-center rounded-2xl transition-all hover:rounded-xl",
                          avatar?.emoji
                            ? cn("text-white", avatar.color || "bg-primary")
                            : "bg-secondary text-secondary-foreground hover:bg-primary hover:text-primary-foreground",
                          isSelected &&
                            (avatar?.emoji
                              ? "rounded-xl ring-2 ring-ring ring-offset-2 ring-offset-sidebar"
                              : "rounded-xl bg-primary text-primary-foreground")
                        )}
                        title={item.name}
                        aria-label={item.name}
                        onClick={() => onSelectProject(item.id)}
                      >
                        {avatar?.emoji ? (
                          <span className="text-xl">{avatar.emoji}</span>
                        ) : (
                          <span className="text-lg font-semibold">{initials(item.name)}</span>
                        )}
                        {isSelected && (
                          <div className="absolute -left-3 h-10 w-1 rounded-r-full bg-foreground" />
                        )}
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="right">{item.name}</TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          </ScrollArea>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className={cn(
                  "flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-secondary text-muted-foreground transition-all hover:rounded-xl hover:bg-green-600 hover:text-white",
                  projectDraftOpen && "rounded-xl bg-green-600 text-white"
                )}
                title="Create project"
                aria-label="Create project"
                onClick={openCreateProject}
              >
                <Plus className="h-5 w-5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">Create project</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
                title={theme === "dark" ? "Light mode" : "Dark mode"}
                aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
                onClick={onToggleTheme}
              >
                {theme === "dark" ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">
              {theme === "dark" ? "Light mode" : "Dark mode"}
            </TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
                title="Log out"
                aria-label="Log out"
                onClick={onLogout}
              >
                <LogOut className="h-5 w-5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">Log out</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>

      {/* Main Content */}
      <div className="relative min-h-0 min-w-0 flex-1">
        <ResizablePanelGroup
          direction="horizontal"
          className="h-full"
          aria-hidden={projectFilesOpen || undefined}
          inert={projectFilesOpen || undefined}
        >
          {/* Channel Sidebar */}
          <ResizablePanel defaultSize={18} minSize={15} maxSize={25}>
            <div className="flex h-full min-h-0 flex-col bg-sidebar">
              {/* Workspace Header */}
              <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
                <h2 className="truncate text-base font-semibold">
                  {project?.name ?? "No project"}
                </h2>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  title="Project settings"
                  aria-label="Project settings"
                  disabled={!project}
                  onClick={openProjectSettings}
                >
                  <ChevronDown className="h-4 w-4" />
                </Button>
              </div>

              <ScrollArea className="min-h-0 flex-1">
                <div className="px-2 py-3">
                  {/* Channels */}
                  <ChannelList
                    channels={channels}
                    selectedChannelID={selectedChannel?.id}
                    metricsActive={mainView === "metrics"}
                    onSelect={selectSidebarChannel}
                    onOpenMetrics={openMetrics}
                    onCreate={() => setChannelDraftOpen(true)}
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
                    onClick={openTasks}
                  >
                    <CalendarClock className="h-4 w-4 shrink-0" />
                    <span className="min-w-0 truncate">Tasks</span>
                  </button>

                  {/* Agents Section */}
                  <AgentsSidebar
                    agents={activeAgents}
                    boundAgents={boundAgents}
                    contextLoading={contextLoading}
                    onOpenPanel={(agentID) => {
                      if (agentID) setFocusedAgentID(agentID);
                      setMembersPanelOpen(false);
                      setAgentPanelOpen(true);
                    }}
                    onCreateAgent={() => setAgentDraftOpen(true)}
                  />
                </div>
              </ScrollArea>

              {/* User Info */}
              <div className="flex shrink-0 items-center gap-2 border-t border-border bg-sidebar p-2">
                <Avatar className="h-8 w-8">
                  <AvatarFallback className="bg-primary text-primary-foreground text-xs">
                    {initials(user.display_name)}
                  </AvatarFallback>
                </Avatar>
                <div className="flex-1 truncate">
                  <p className="text-sm font-medium">{user.display_name}</p>
                  <p className="text-xs text-muted-foreground">online</p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  title="User settings"
                  aria-label="User settings"
                  onClick={openAccountSettings}
                >
                  <Settings className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </ResizablePanel>

        <ResizableHandle withHandle />

        {/* Message Area */}
        <ResizablePanel defaultSize={agentPanelOpen ? 37 : membersPanelOpen ? 62 : 82}>
          <div className="flex h-full min-h-0 flex-1 flex-col bg-background">
            {/* Channel Header */}
            <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
              <div className="flex items-center gap-2">
                {selectedChannel?.type === "thread" && activeThread ? (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    title="Back to posts"
                    aria-label="Back to posts"
                    onClick={() => onSelectChannel(selectedChannel)}
                  >
                    <ArrowLeft className="h-4 w-4" />
                  </Button>
                ) : null}
                {selectedChannel?.type === "thread" && !activeThread ? (
                  <Rows3 className="h-5 w-5 text-muted-foreground" />
                ) : boundAgents.length === 1 ? (
                  <Bot className={cn("h-5 w-5", agentToneColor(boundAgents[0].agent.kind))} />
                ) : (
                  <Hash className="h-5 w-5 text-muted-foreground" />
                )}
                <div>
                  <h1 className="text-sm font-semibold">{title}</h1>
                  <p className="text-xs text-muted-foreground">{subtitle}</p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {activeThread && (
                  <>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
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
                      className="h-8 w-8 text-muted-foreground hover:text-destructive"
                      title="Delete post"
                      aria-label="Delete post"
                      disabled={threadActionPending}
                      onClick={deleteActiveThread}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </>
                )}
                {activeConversation && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Activity className="h-3.5 w-3.5" />
                    {activityLabel}
                  </span>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-8 w-8", mainView === "metrics" && "bg-accent")}
                  title="Metrics"
                  aria-label="Metrics"
                  aria-pressed={mainView === "metrics"}
                  disabled={!project && !selectedChannel}
                  onClick={openMetrics}
                >
                  <BarChart3 className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-8 w-8", mainView === "tasks" && "bg-accent")}
                  title="Tasks"
                  aria-label="Tasks"
                  aria-pressed={mainView === "tasks"}
                  disabled={!project}
                  onClick={openTasks}
                >
                  <CalendarClock className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  title="Project files"
                  aria-label="Project files"
                  aria-pressed="false"
                  disabled={!projectWorkspace?.id}
                  onClick={toggleProjectFiles}
                >
                  <FolderOpen className="h-4 w-4" />
                </Button>
                {terminalAllowed ? (
                  <Button
                    variant="ghost"
                    size="icon"
                    className={cn("h-8 w-8", terminalOpen && "bg-accent")}
                    title="Terminal"
                    aria-label="Terminal"
                    aria-pressed={terminalOpen}
                    disabled={!projectWorkspace?.id}
                    onClick={() => setTerminalOpen((open) => !open)}
                  >
                    <SquareTerminal className="h-4 w-4" />
                  </Button>
                ) : null}
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-8 w-8", membersPanelOpen && "bg-accent")}
                  title="Members"
                  aria-label="Members"
                  onClick={() => setMembersPanelOpen((open) => !open)}
                >
                  <UserRound className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className={cn("h-8 w-8", agentPanelOpen && "bg-accent")}
                  title="Agent settings"
                  aria-label="Agent settings"
                  onClick={() => {
                    setMembersPanelOpen(false);
                    setAgentPanelOpen((open) => !open);
                  }}
                >
                  <Settings className="h-4 w-4" />
                </Button>
              </div>
            </div>

            <div ref={terminalResizeContainerRef} className="flex min-h-0 flex-1 flex-col">
              <div className="flex min-h-0 flex-1 flex-col">
                {mainView === "metrics" ? (
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
                    onRetryMessage={onRetryMessage}
                    onLoadOlderMessages={onLoadOlderMessages}
                    onRespondToQuestion={onRespondToQuestion}
                    onSteerQueuedPrompt={onSteerQueuedPrompt}
              onDeleteQueuedPrompt={onDeleteQueuedPrompt}
                    onMessageSent={onMessageSent}
                    workspacePath={projectWorkspace?.path}
                    onOpenWorkspacePath={openWorkspacePath}
                  />
                )}
              </div>
              {terminalOpen && terminalAllowed && projectWorkspace?.id ? (
                <div
                  className="relative min-h-56 shrink-0 overflow-hidden border-t border-border"
                  style={{ height: `${terminalHeightPct}%` }}
                >
                  <div
                    className="absolute inset-x-0 -top-1 z-10 flex h-2 cursor-row-resize items-start justify-center"
                    onPointerDown={startTerminalResize}
                    role="separator"
                    aria-orientation="horizontal"
                    aria-label="Resize terminal"
                  >
                    <span className="mt-0.5 h-1 w-12 rounded-full bg-border" />
                  </div>
                  <TerminalDockBoundary
                                          workspace={projectWorkspace}
                      theme={theme}
                      className="h-full"
                      onClose={() => setTerminalOpen(false)}
                    />
                  
                </div>
              ) : null}
            </div>
          </div>
        </ResizablePanel>

        {/* Members Panel */}
        {membersPanelOpen && !agentPanelOpen && !projectFilesOpen && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel defaultSize={20} minSize={15} maxSize={30}>
              <MembersPanel
                agents={activeAgents}
                boundAgents={boundAgents}
                projectWorkspace={projectWorkspace}
                selectedChannel={selectedChannel}
                onSaveChannelAgents={onSaveChannelAgents}
                onClose={() => setMembersPanelOpen(false)}
              />
            </ResizablePanel>
          </>
        )}

        {/* Agent Panel */}
        {agentPanelOpen && !projectFilesOpen && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel defaultSize={45} minSize={32} maxSize={60}>
              <AgentDetailsPanel
                selectedChannel={selectedChannel}
                projectWorkspace={projectWorkspace}
                agents={activeAgents}
                boundAgents={boundAgents}
                selectedAgent={selectedAgent}
                onUpdateAgent={onUpdateAgent}
                onDeleteAgent={onDeleteAgent}
                toolUpdates={toolUpdates}
                toolUpdatesLoading={toolUpdatesLoading}
                onCheckToolUpdates={onCheckToolUpdates}
                onRunToolUpdate={onRunToolUpdate}
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
                onClose={() => setAgentPanelOpen(false)}
                theme={theme}
              />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
      <ProjectFilesOverlay
        open={projectFilesOpen}
        controller={projectFilesController}
        theme={theme}
        treeCollapsed={projectFileTreeCollapsed}
        setTreeCollapsed={setProjectFileTreeCollapsed}
        onClose={toggleProjectFiles}
        onChangeSelected={() => setMobileProjectFilesView("editor")}
      />
      </div>
      </div>
  );
}
