import {
  type FormEvent,
  type MouseEvent as ReactMouseEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { Pencil, Plus, RefreshCw, SquareTerminal, Trash2, X } from "lucide-react";
import { deleteWorkspaceTerminal, renameWorkspaceTerminal, workspaceTerminals } from "@/api/client";
import type { TerminalSessionSummary } from "@/api/types";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { TerminalPane } from "./terminal/TerminalPane";
import {
  appendUnique,
  compactTerminalPanesAfterClose,
  createTerminalPane,
  ensurePane,
  splitTerminalPanes,
  terminalConnectionTarget,
  terminalReconnectTarget,
} from "./terminal/paneState";
import type { TerminalDockProps, TerminalPaneState, TerminalSplitSide } from "./terminal/types";
import { leftTerminalPaneID, primaryTerminalPaneID, rightTerminalPaneID } from "./terminal/types";
import { terminalClientID } from "./terminal/utils";

export function TerminalDock({ workspace, theme, className, onClose }: TerminalDockProps) {
  const [sessions, setSessions] = useState<TerminalSessionSummary[]>([]);
  const [terminalPanes, setTerminalPanes] = useState<TerminalPaneState[]>(() => [
    createTerminalPane(primaryTerminalPaneID),
  ]);
  const [activePaneID, setActivePaneID] = useState(primaryTerminalPaneID);
  const [loading, setLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [renameSession, setRenameSession] = useState<TerminalSessionSummary | null>(null);
  const [renameTitle, setRenameTitle] = useState("");
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<string | null>(null);
  const [dropSide, setDropSide] = useState<TerminalSplitSide | null>(null);
  const [layoutVersion, setLayoutVersion] = useState(0);
  const autoCreateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const layoutTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const splitDropzoneRef = useRef<HTMLDivElement | null>(null);
  const tabDragCleanupRef = useRef<(() => void) | null>(null);
  const tabDragRef = useRef<{
    dragging: boolean;
    startX: number;
    startY: number;
    terminalID: string;
  } | null>(null);
  const suppressTabClickRef = useRef(false);
  const workspaceID = workspace?.id;
  const splitMode = terminalPanes.length > 1;
  const activePane = terminalPanes.find((pane) => pane.id === activePaneID) ?? terminalPanes[0];

  const clearAutoCreateTimer = useCallback(() => {
    if (autoCreateTimerRef.current) {
      clearTimeout(autoCreateTimerRef.current);
      autoCreateTimerRef.current = null;
    }
  }, []);

  const scheduleTerminalLayout = useCallback(() => {
    if (layoutTimerRef.current) {
      clearTimeout(layoutTimerRef.current);
    }
    layoutTimerRef.current = setTimeout(() => {
      layoutTimerRef.current = null;
      setLayoutVersion((version) => version + 1);
    }, 80);
  }, []);

  const upsertSession = useCallback((session: TerminalSessionSummary) => {
    setSessions((current) => {
      const next = current.some((item) => item.id === session.id)
        ? current.map((item) => (item.id === session.id ? session : item))
        : [...current, session];
      return next.sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
    });
  }, []);

  const openNewTerminalInPane = useCallback((targetPaneID: string) => {
    clearAutoCreateTimer();
    setActionError(null);
    const clientID = terminalClientID();
    setActivePaneID(targetPaneID);
    setTerminalPanes((current) => ensurePane(current, targetPaneID).map((pane) => (
      pane.id === targetPaneID
        ? { ...pane, activeTerminalID: "", connectionTarget: { key: `new:${clientID}`, clientID } }
        : pane
    )));
  }, [clearAutoCreateTimer]);

  const openNewTerminal = useCallback((paneID?: string) => {
    openNewTerminalInPane(paneID ?? activePaneID);
  }, [activePaneID, openNewTerminalInPane]);

  const scheduleNewTerminal = useCallback((paneID?: string) => {
    clearAutoCreateTimer();
    autoCreateTimerRef.current = setTimeout(() => {
      autoCreateTimerRef.current = null;
      openNewTerminalInPane(paneID ?? primaryTerminalPaneID);
    }, 120);
  }, [clearAutoCreateTimer, openNewTerminalInPane]);

  const openSession = useCallback((paneID: string, terminalID: string) => {
    setActionError(null);
    setActivePaneID(paneID);
    setTerminalPanes((current) => ensurePane(current, paneID).map((pane) => (
      pane.id === paneID
        ? { ...pane, activeTerminalID: terminalID, connectionTarget: terminalConnectionTarget(terminalID) }
        : pane
    )));
  }, []);

  const reconnectSession = useCallback((paneID: string, terminalID: string) => {
    setActionError(null);
    setActivePaneID(paneID);
    setTerminalPanes((current) => ensurePane(current, paneID).map((pane) => (
      pane.id === paneID
        ? { ...pane, activeTerminalID: terminalID, connectionTarget: terminalReconnectTarget(terminalID) }
        : pane
    )));
  }, []);

  const loadSessions = useCallback(async () => {
    if (!workspaceID) {
      setSessions([]);
      setTerminalPanes([createTerminalPane(primaryTerminalPaneID)]);
      setActivePaneID(primaryTerminalPaneID);
      return;
    }
    setLoading(true);
    setActionError(null);
    try {
      const next = await workspaceTerminals(workspaceID);
      setSessions(next);
      if (next.length > 0) {
        const preferred = next.find((item) => item.status === "running") ?? next[next.length - 1];
        setTerminalPanes([
          createTerminalPane(
            primaryTerminalPaneID,
            next.map((item) => item.id),
            preferred.id,
            terminalConnectionTarget(preferred.id)
          ),
        ]);
        setActivePaneID(primaryTerminalPaneID);
      } else {
        setTerminalPanes([createTerminalPane(primaryTerminalPaneID)]);
        setActivePaneID(primaryTerminalPaneID);
        scheduleNewTerminal(primaryTerminalPaneID);
      }
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to load terminals");
    } finally {
      setLoading(false);
    }
  }, [scheduleNewTerminal, workspaceID]);

  useEffect(() => {
    void loadSessions();
  }, [loadSessions]);

  useEffect(() => clearAutoCreateTimer, [clearAutoCreateTimer]);

  useEffect(() => () => {
    tabDragCleanupRef.current?.();
    if (layoutTimerRef.current) {
      clearTimeout(layoutTimerRef.current);
    }
  }, []);

  async function closeTerminal(terminalID: string) {
    if (!workspaceID || !terminalID) return;
    setActionError(null);
    try {
      await deleteWorkspaceTerminal(workspaceID, terminalID);
      const next = sessions.filter((item) => item.id !== terminalID);
      setSessions(next);
      if (next.length === 0) {
        const clientID = terminalClientID();
        setTerminalPanes([
          createTerminalPane(primaryTerminalPaneID, [], "", { key: `new:${clientID}`, clientID }),
        ]);
        setActivePaneID(primaryTerminalPaneID);
        return;
      }
      setTerminalPanes((current) => compactTerminalPanesAfterClose(current, next, terminalID));
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Failed to close terminal");
    }
  }

  async function closeActiveTerminal() {
    await closeTerminal(activePane?.activeTerminalID ?? "");
  }

  function openRenameDialog(session: TerminalSessionSummary) {
    setRenameSession(session);
    setRenameTitle(session.title);
    setRenameError(null);
  }

  async function submitRename(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspaceID || !renameSession) return;
    const title = renameTitle.trim();
    if (!title) {
      setRenameError("Terminal name is required");
      return;
    }
    setRenamePending(true);
    setRenameError(null);
    try {
      const updated = await renameWorkspaceTerminal(workspaceID, renameSession.id, title);
      upsertSession(updated);
      setRenameSession(null);
      setRenameTitle("");
    } catch (err) {
      setRenameError(err instanceof Error ? err.message : "Failed to rename terminal");
    } finally {
      setRenamePending(false);
    }
  }

  function moveTerminalToSplit(terminalID: string, side: TerminalSplitSide) {
    if (!sessions.some((session) => session.id === terminalID)) return;
    const targetPaneID = side === "left" ? leftTerminalPaneID : rightTerminalPaneID;
    setActivePaneID(targetPaneID);
    setTerminalPanes((current) => splitTerminalPanes(current, sessions, terminalID, side));
    scheduleTerminalLayout();
  }

  function splitSideForPoint(clientX: number, clientY: number): TerminalSplitSide | null {
    const rect = splitDropzoneRef.current?.getBoundingClientRect();
    if (!rect) return null;
    if (clientX < rect.left || clientX > rect.right || clientY < rect.top || clientY > rect.bottom) {
      return null;
    }
    return clientX - rect.left < rect.width / 2 ? "left" : "right";
  }

  function startTabMouseDrag(event: ReactMouseEvent<HTMLButtonElement>, terminalID: string) {
    if (event.button !== 0) return;
    tabDragCleanupRef.current?.();
    tabDragRef.current = {
      dragging: false,
      startX: event.clientX,
      startY: event.clientY,
      terminalID,
    };

    const cleanup = () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
      window.removeEventListener("blur", handleWindowBlur);
      tabDragCleanupRef.current = null;
    };
    const finish = (moveTerminal: boolean, clientX: number, clientY: number) => {
      const drag = tabDragRef.current;
      tabDragRef.current = null;
      setDropSide(null);
      cleanup();
      if (!drag?.dragging) return;
      suppressTabClickRef.current = true;
      window.setTimeout(() => {
        suppressTabClickRef.current = false;
      }, 0);
      if (moveTerminal) {
        const side = splitSideForPoint(clientX, clientY);
        if (side) moveTerminalToSplit(drag.terminalID, side);
      }
    };
    const handleMouseMove = (moveEvent: MouseEvent) => {
      const drag = tabDragRef.current;
      if (!drag) return;
      const distance = Math.hypot(moveEvent.clientX - drag.startX, moveEvent.clientY - drag.startY);
      if (!drag.dragging && distance < 6) return;
      drag.dragging = true;
      setDropSide(splitSideForPoint(moveEvent.clientX, moveEvent.clientY));
      moveEvent.preventDefault();
    };
    const handleMouseUp = (upEvent: MouseEvent) => {
      if (!tabDragRef.current) return;
      finish(true, upEvent.clientX, upEvent.clientY);
      upEvent.preventDefault();
    };
    const handleWindowBlur = () => {
      tabDragRef.current = null;
      setDropSide(null);
      cleanup();
    };
    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
    window.addEventListener("blur", handleWindowBlur);
    tabDragCleanupRef.current = cleanup;
  }

  function renderPaneTabs(pane: TerminalPaneState) {
    const paneSessions = pane.terminalIDs
      .map((id) => sessions.find((session) => session.id === id))
      .filter((session): session is TerminalSessionSummary => Boolean(session));

    return (
      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
        {paneSessions.map((session) => (
          <ContextMenu key={session.id}>
            <ContextMenuTrigger asChild>
              <button
                type="button"
                className={cn(
                  "flex h-8 max-w-48 shrink-0 items-center gap-2 rounded-md border px-2 text-xs transition-colors",
                  pane.activeTerminalID === session.id
                    ? "border-border bg-background text-foreground"
                    : "border-transparent text-muted-foreground hover:bg-accent/60 hover:text-foreground"
                )}
                title={session.title}
                aria-label={`Terminal ${session.title}`}
                aria-pressed={pane.activeTerminalID === session.id}
                onClick={(event) => {
                  if (suppressTabClickRef.current) {
                    suppressTabClickRef.current = false;
                    event.preventDefault();
                    return;
                  }
                  openSession(pane.id, session.id);
                }}
                onMouseDown={(event) => startTabMouseDrag(event, session.id)}
              >
                <SquareTerminal className="h-3.5 w-3.5 shrink-0" />
                <span className="truncate">{session.title}</span>
                {session.status === "exited" ? (
                  <span className="shrink-0 rounded bg-muted px-1 text-[10px] text-muted-foreground">
                    exited
                  </span>
                ) : null}
              </button>
            </ContextMenuTrigger>
            <ContextMenuContent>
              <ContextMenuItem onSelect={() => openRenameDialog(session)}>
                <Pencil className="h-3.5 w-3.5" />
                Rename terminal
              </ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuItem variant="destructive" onSelect={() => void closeTerminal(session.id)}>
                <Trash2 className="h-3.5 w-3.5" />
                Close terminal
              </ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        ))}
        {pane.connectionTarget && !pane.activeTerminalID ? (
          <div className="flex h-8 shrink-0 items-center gap-2 rounded-md border border-border bg-background px-2 text-xs text-foreground">
            <SquareTerminal className="h-3.5 w-3.5" />
            New terminal
          </div>
        ) : null}
      </div>
    );
  }

  function renderPaneContent(pane: TerminalPaneState) {
    const activeSession = sessions.find((session) => session.id === pane.activeTerminalID);
    return (
      <div
        className={cn(
          "relative min-h-0 flex-1",
          theme === "light" ? "bg-[#f8fafc]" : "bg-[#111315]"
        )}
      >
        {!workspaceID ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            No project workspace
          </div>
        ) : pane.connectionTarget ? (
          <TerminalPane
            key={pane.connectionTarget.key}
            workspaceID={workspaceID}
            initialTerminalID={pane.connectionTarget.terminalID}
            clientID={pane.connectionTarget.clientID}
            session={activeSession}
            theme={theme}
            layoutVersion={layoutVersion}
            onReady={(session) => {
              upsertSession(session);
              setActivePaneID(pane.id);
              setTerminalPanes((current) => current.map((item) => (
                item.id === pane.id
                  && (item.activeTerminalID !== session.id || !item.terminalIDs.includes(session.id))
                  ? {
                      ...item,
                      terminalIDs: appendUnique(item.terminalIDs, session.id),
                      activeTerminalID: session.id,
                    }
                  : item
              )));
            }}
            onSessionChange={upsertSession}
            onReconnect={() => {
              const terminalID = pane.activeTerminalID || activeSession?.id;
              if (terminalID) reconnectSession(pane.id, terminalID);
            }}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            {loading ? "Loading terminals..." : "No terminal selected"}
          </div>
        )}
      </div>
    );
  }

  function renderSplitPane(pane: TerminalPaneState) {
    return (
      <div
        className={cn(
          "flex h-full min-h-0 flex-col border-border",
          activePaneID === pane.id && "bg-accent/10"
        )}
        data-testid="terminal-pane-group"
        onPointerDown={() => setActivePaneID(pane.id)}
      >
        <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-sidebar px-2">
          {renderPaneTabs(pane)}
        </div>
        {renderPaneContent(pane)}
      </div>
    );
  }

  return (
    <>
      <section
        className={cn("flex min-h-0 flex-1 flex-col bg-background", className)}
        aria-label="Terminal"
        data-testid="terminal-dock"
      >
        <div className="flex h-11 shrink-0 items-center gap-2 border-b border-border bg-sidebar px-2">
          {splitMode ? (
            <div className="flex min-w-0 flex-1 items-center gap-2 px-1 text-xs font-medium text-muted-foreground">
              <SquareTerminal className="h-3.5 w-3.5" />
              <span>Terminals</span>
            </div>
          ) : (
            renderPaneTabs(terminalPanes[0])
          )}
          {actionError ? <span className="max-w-72 truncate text-xs text-destructive">{actionError}</span> : null}
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-8 w-8 shrink-0"
            title="Refresh terminals"
            aria-label="Refresh terminals"
            disabled={!workspaceID || loading}
            onClick={() => void loadSessions()}
          >
            <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-8 w-8 shrink-0"
            title="New terminal"
            aria-label="New terminal"
            disabled={!workspaceID}
            onClick={() => openNewTerminal(activePane?.id)}
          >
            <Plus className="h-4 w-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-8 w-8 shrink-0 text-muted-foreground hover:text-destructive"
            title="Close terminal"
            aria-label="Close terminal"
            disabled={!workspaceID || !activePane?.activeTerminalID}
            onClick={() => void closeActiveTerminal()}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
          {onClose ? (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0"
              title="Hide terminal"
              aria-label="Hide terminal"
              onClick={onClose}
            >
              <X className="h-4 w-4" />
            </Button>
          ) : null}
        </div>

        <div
          ref={splitDropzoneRef}
          className="relative flex min-h-0 flex-1 flex-col"
          data-testid="terminal-split-dropzone"
        >
          {splitMode ? (
            <ResizablePanelGroup
              direction="horizontal"
              className="h-full min-h-0"
              autoSaveId="agentx-terminal-split"
              onLayout={scheduleTerminalLayout}
            >
              <ResizablePanel id={leftTerminalPaneID} order={1} defaultSize={50} minSize={25}>
                {renderSplitPane(terminalPanes[0])}
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel id={rightTerminalPaneID} order={2} defaultSize={50} minSize={25}>
                {renderSplitPane(terminalPanes[1])}
              </ResizablePanel>
            </ResizablePanelGroup>
          ) : (
            renderPaneContent(terminalPanes[0])
          )}
          {dropSide ? (
            <div
              className={cn(
                "pointer-events-none absolute inset-y-0 z-20 border-2 border-primary/70 bg-primary/10",
                dropSide === "left" ? "left-0 w-1/2" : "right-0 w-1/2"
              )}
            />
          ) : null}
        </div>
      </section>
      <Dialog
        open={renameSession !== null}
        onOpenChange={(open) => {
          if (!open) {
            setRenameSession(null);
            setRenameTitle("");
            setRenameError(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename terminal</DialogTitle>
            <DialogDescription>Update the terminal tab name.</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={submitRename}>
            <div className="grid gap-2">
              <Label htmlFor="terminal-title">Name</Label>
              <Input
                id="terminal-title"
                value={renameTitle}
                maxLength={80}
                autoFocus
                onChange={(event) => {
                  setRenameTitle(event.target.value);
                  setRenameError(null);
                }}
              />
              {renameError ? <p className="text-sm text-destructive">{renameError}</p> : null}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={renamePending}
                onClick={() => setRenameSession(null)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={renamePending || !renameTitle.trim()}>
                {renamePending ? "Saving..." : "Save"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
