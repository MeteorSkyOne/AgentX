import {
  type FormEvent,
  type MouseEvent as ReactMouseEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import {
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  SquareTerminal,
  Trash2,
  X,
} from "lucide-react";
import {
  deleteWorkspaceTerminal,
  renameWorkspaceTerminal,
  workspaceTerminalWebSocketURL,
  workspaceTerminals,
} from "@/api/client";
import type { TerminalFrame, TerminalSessionSummary, Workspace } from "@/api/types";
import type { ThemeMode } from "@/theme";
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
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";

interface TerminalDockProps {
  workspace?: Workspace;
  theme: ThemeMode;
  className?: string;
  onClose?: () => void;
}

type TerminalConnectionTarget = {
  key: string;
  terminalID?: string;
  clientID?: string;
};

type TerminalConnectionState = "connecting" | "connected" | "disconnected" | "exited" | "error";
type TerminalSplitSide = "left" | "right";

type TerminalPaneState = {
  id: string;
  terminalIDs: string[];
  activeTerminalID: string;
  connectionTarget: TerminalConnectionTarget | null;
};

const maxTerminalClipboardBytes = 1024 * 1024;
const primaryTerminalPaneID = "terminal-pane-primary";
const leftTerminalPaneID = "terminal-pane-left";
const rightTerminalPaneID = "terminal-pane-right";
type TerminalFitMode = "debounced" | "immediate";

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
          className="relative min-h-0 flex-1"
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

function createTerminalPane(
  id: string,
  terminalIDs: string[] = [],
  activeTerminalID = "",
  connectionTarget: TerminalConnectionTarget | null = null
): TerminalPaneState {
  return { id, terminalIDs, activeTerminalID, connectionTarget };
}

function ensurePane(panes: TerminalPaneState[], paneID: string): TerminalPaneState[] {
  if (panes.some((pane) => pane.id === paneID)) {
    return panes;
  }
  return [...panes, createTerminalPane(paneID)];
}

function terminalConnectionTarget(terminalID: string): TerminalConnectionTarget {
  return { key: `session:${terminalID}`, terminalID };
}

function terminalReconnectTarget(terminalID: string): TerminalConnectionTarget {
  return { key: `session:${terminalID}:reconnect:${terminalClientID()}`, terminalID };
}

function terminalNewConnectionTarget(): TerminalConnectionTarget {
  const clientID = terminalClientID();
  return { key: `new:${clientID}`, clientID };
}

function appendUnique(values: string[], value: string): string[] {
  return values.includes(value) ? values : [...values, value];
}

function preferredSessionForIDs(
  sessions: TerminalSessionSummary[],
  terminalIDs: string[]
): TerminalSessionSummary | undefined {
  const paneSessions = terminalIDs
    .map((id) => sessions.find((session) => session.id === id))
    .filter((session): session is TerminalSessionSummary => Boolean(session));
  return paneSessions.find((session) => session.status === "running") ?? paneSessions[paneSessions.length - 1];
}

function normalizePaneSelection(
  pane: TerminalPaneState,
  sessions: TerminalSessionSummary[]
): TerminalPaneState {
  if (pane.activeTerminalID && pane.terminalIDs.includes(pane.activeTerminalID)) {
    return pane;
  }
  const fallback = preferredSessionForIDs(sessions, pane.terminalIDs);
  if (!fallback) {
    return { ...pane, activeTerminalID: "", connectionTarget: null };
  }
  return {
    ...pane,
    activeTerminalID: fallback.id,
    connectionTarget: terminalConnectionTarget(fallback.id),
  };
}

function ensurePaneHasContent(pane: TerminalPaneState): TerminalPaneState {
  if (pane.terminalIDs.length > 0 || pane.connectionTarget) {
    return pane;
  }
  return { ...pane, activeTerminalID: "", connectionTarget: terminalNewConnectionTarget() };
}

function compactTerminalPanesAfterClose(
  panes: TerminalPaneState[],
  sessions: TerminalSessionSummary[],
  terminalID: string
): TerminalPaneState[] {
  const next = panes
    .map((pane) => normalizePaneSelection({
      ...pane,
      terminalIDs: pane.terminalIDs.filter((id) => id !== terminalID),
      activeTerminalID: pane.activeTerminalID === terminalID ? "" : pane.activeTerminalID,
      connectionTarget: pane.connectionTarget?.terminalID === terminalID ? null : pane.connectionTarget,
    }, sessions))
    .filter((pane) => pane.terminalIDs.length > 0 || pane.connectionTarget);
  return next.length > 0 ? next.slice(0, 2) : [createTerminalPane(primaryTerminalPaneID)];
}

function splitTerminalPanes(
  panes: TerminalPaneState[],
  sessions: TerminalSessionSummary[],
  terminalID: string,
  side: TerminalSplitSide
): TerminalPaneState[] {
  const targetPaneID = side === "left" ? leftTerminalPaneID : rightTerminalPaneID;
  const otherPaneID = side === "left" ? rightTerminalPaneID : leftTerminalPaneID;
  const targetConnection = terminalConnectionTarget(terminalID);

  if (panes.length <= 1) {
    const remainingIDs = (panes[0]?.terminalIDs ?? []).filter((id) => id !== terminalID);
    const remainingFallback = preferredSessionForIDs(sessions, remainingIDs);
    const sourcePane = ensurePaneHasContent(createTerminalPane(
      otherPaneID,
      remainingIDs,
      remainingFallback?.id ?? "",
      remainingFallback ? terminalConnectionTarget(remainingFallback.id) : null
    ));
    const targetPane = createTerminalPane(targetPaneID, [terminalID], terminalID, targetConnection);
    return side === "left" ? [targetPane, sourcePane] : [sourcePane, targetPane];
  }

  const leftPane = {
    ...panes[0],
    id: leftTerminalPaneID,
    terminalIDs: panes[0].terminalIDs.filter((id) => id !== terminalID),
  };
  const rightPane = {
    ...panes[1],
    id: rightTerminalPaneID,
    terminalIDs: panes[1].terminalIDs.filter((id) => id !== terminalID),
  };
  const normalized = [leftPane, rightPane].map((pane) => normalizePaneSelection(pane, sessions));
  const targetIndex = targetPaneID === leftTerminalPaneID ? 0 : 1;
  normalized[targetIndex] = {
    ...normalized[targetIndex],
    terminalIDs: appendUnique(normalized[targetIndex].terminalIDs, terminalID),
    activeTerminalID: terminalID,
    connectionTarget: targetConnection,
  };
  return normalized.map(ensurePaneHasContent);
}

function TerminalPane({
  workspaceID,
  initialTerminalID,
  clientID,
  session,
  theme,
  layoutVersion,
  onReady,
  onSessionChange,
  onReconnect,
}: {
  workspaceID: string;
  initialTerminalID?: string;
  clientID?: string;
  session?: TerminalSessionSummary;
  theme: ThemeMode;
  layoutVersion: number;
  onReady: (session: TerminalSessionSummary) => void;
  onSessionChange: (session: TerminalSessionSummary) => void;
  onReconnect: () => void;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const resizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const resizeSendTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const fitTerminalRef = useRef<((force?: boolean, mode?: TerminalFitMode) => void) | null>(null);
  const attachedRef = useRef(false);
  const exitedRef = useRef(false);
  const errorRef = useRef(false);
  const sessionRef = useRef(session);
  const onReadyRef = useRef(onReady);
  const onSessionChangeRef = useRef(onSessionChange);
  const [state, setState] = useState<TerminalConnectionState>("connecting");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    sessionRef.current = session;
  }, [session]);

  useEffect(() => {
    onReadyRef.current = onReady;
    onSessionChangeRef.current = onSessionChange;
  }, [onReady, onSessionChange]);

  useLayoutEffect(() => {
    const run = () => fitTerminalRef.current?.(true, "immediate");
    run();
    const first = window.setTimeout(run, 0);
    const second = window.setTimeout(run, 120);
    return () => {
      window.clearTimeout(first);
      window.clearTimeout(second);
    };
  }, [layoutVersion]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let stopped = false;
    exitedRef.current = false;
    errorRef.current = false;
    const terminal = new XTerm({
      cursorBlink: true,
      convertEol: false,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      fontSize: 13,
      scrollback: 10000,
      scrollOnEraseInDisplay: true,
      macOptionClickForcesSelection: true,
      theme: terminalTheme(theme),
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(container);
    terminal.focus();
    let lastCopiedSelection = "";
    let lastFitHeight = 0;
    let lastFitWidth = 0;
    let lastSentCols = 0;
    let lastSentRows = 0;
    let pendingResize: { cols: number; rows: number } | null = null;

    const send = (payload: unknown) => {
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) return false;
      socket.send(JSON.stringify(payload));
      return true;
    };

    const scheduleResizeIfChanged = (cols = terminal.cols, rows = terminal.rows) => {
      if (!attachedRef.current) return;
      if (cols === lastSentCols && rows === lastSentRows) return;
      pendingResize = { cols, rows };
      if (resizeSendTimerRef.current) {
        clearTimeout(resizeSendTimerRef.current);
      }
      resizeSendTimerRef.current = setTimeout(() => {
        resizeSendTimerRef.current = null;
        const next = pendingResize;
        pendingResize = null;
        if (!next || !attachedRef.current) return;
        if (next.cols === lastSentCols && next.rows === lastSentRows) return;
        lastSentCols = next.cols;
        lastSentRows = next.rows;
        send({ type: "resize", cols: next.cols, rows: next.rows });
      }, 120);
    };

    const sendResizeNowIfChanged = (cols = terminal.cols, rows = terminal.rows) => {
      if (!attachedRef.current) return;
      if (resizeSendTimerRef.current) {
        clearTimeout(resizeSendTimerRef.current);
        resizeSendTimerRef.current = null;
      }
      pendingResize = null;
      if (cols === lastSentCols && rows === lastSentRows) return;
      lastSentCols = cols;
      lastSentRows = rows;
      send({ type: "resize", cols, rows });
    };

    const runFit = (mode: TerminalFitMode) => {
      try {
        fitAddon.fit();
        if (mode === "immediate") {
          sendResizeNowIfChanged();
        } else {
          scheduleResizeIfChanged();
        }
      } catch {
        // xterm cannot fit until the container has measurable dimensions.
      }
    };

    const fitTerminal = (force = false, mode: TerminalFitMode = "debounced") => {
      const rect = container.getBoundingClientRect();
      const width = Math.round(rect.width);
      const height = Math.round(rect.height);
      if (!force && width === lastFitWidth && height === lastFitHeight) return;
      lastFitWidth = width;
      lastFitHeight = height;
      if (resizeTimerRef.current) {
        clearTimeout(resizeTimerRef.current);
        resizeTimerRef.current = null;
      }
      if (mode === "immediate") {
        runFit(mode);
        return;
      }
      resizeTimerRef.current = setTimeout(() => {
        resizeTimerRef.current = null;
        runFit(mode);
      }, 60);
    };
    fitTerminalRef.current = fitTerminal;

    const observer = typeof ResizeObserver !== "undefined"
      ? new ResizeObserver(() => fitTerminal())
      : null;
    observer?.observe(container);
    setTimeout(() => fitTerminal(true), 0);

    const dataDisposable = terminal.onData((data) => {
      send({
        type: "input",
        data: bytesToBase64(new TextEncoder().encode(data)),
      });
    });
    const binaryDisposable = terminal.onBinary((data) => {
      send({
        type: "input",
        data: bytesToBase64(binaryStringToBytes(data)),
      });
    });
    const selectionDisposable = terminal.onSelectionChange(() => {
      if (!terminal.hasSelection()) {
        lastCopiedSelection = "";
      }
    });
    const osc52Disposable = terminal.parser.registerOscHandler(52, (data) => {
      const text = terminalOsc52ClipboardText(data);
      if (text === null) return true;
      void copyTerminalText(text).catch(() => undefined);
      return true;
    });
    const copySelection = () => {
      const text = terminal.getSelection();
      if (!text || text === lastCopiedSelection) return;
      lastCopiedSelection = text;
      void copyTerminalText(text).catch(() => {
        lastCopiedSelection = "";
      });
    };
    const copySelectionAfterMouseUp = (event: MouseEvent) => {
      if (event.button !== 0) return;
      window.setTimeout(copySelection, 0);
    };
    container.addEventListener("mouseup", copySelectionAfterMouseUp);

    const socket = new WebSocket(workspaceTerminalWebSocketURL(workspaceID));
    socketRef.current = socket;
    setState("connecting");
    setError(null);

    socket.addEventListener("open", () => {
      if (stopped) return;
      try {
        fitAddon.fit();
      } catch {
        // Best effort. The ResizeObserver will retry after layout settles.
      }
      lastSentCols = terminal.cols;
      lastSentRows = terminal.rows;
      pendingResize = null;
      if (resizeSendTimerRef.current) {
        clearTimeout(resizeSendTimerRef.current);
        resizeSendTimerRef.current = null;
      }
      socket.send(JSON.stringify({
        type: "attach",
        terminal_id: initialTerminalID ?? "",
        client_id: clientID ?? "",
        cols: terminal.cols,
        rows: terminal.rows,
        since_seq: 0,
      }));
    });

    socket.addEventListener("message", (event) => {
      if (stopped) return;
      try {
        const frame = JSON.parse(String(event.data)) as TerminalFrame;
        if (frame.type === "ready" && frame.session) {
          attachedRef.current = true;
          setState(frame.session.status === "exited" ? "exited" : "connected");
          exitedRef.current = frame.session.status === "exited";
          onReadyRef.current(frame.session);
          fitTerminal(true, "immediate");
          return;
        }
        if (frame.type === "output" && frame.data) {
          terminal.write(base64ToBytes(frame.data));
          return;
        }
        if (frame.type === "history_truncated") {
          terminal.writeln("\r\n\x1b[33mTerminal history was truncated.\x1b[0m");
          return;
        }
        if (frame.type === "exit") {
          exitedRef.current = true;
          setState("exited");
          const currentSession = sessionRef.current;
          const nextSession = currentSession
            ? {
                ...currentSession,
                status: "exited" as const,
                exit_code: frame.exit_code,
                error: frame.error,
                exited_at: new Date().toISOString(),
                updated_at: new Date().toISOString(),
              }
            : undefined;
          if (nextSession) onSessionChangeRef.current(nextSession);
          const label = frame.error || (typeof frame.exit_code === "number" ? `exited with status ${frame.exit_code}` : "exited");
          terminal.writeln(`\r\n\x1b[90m[terminal ${label}]\x1b[0m`);
          return;
        }
        if (frame.type === "error") {
          errorRef.current = true;
          setState("error");
          setError(frame.error ?? "Terminal error");
          terminal.writeln(`\r\n\x1b[31m${frame.error ?? "Terminal error"}\x1b[0m`);
        }
      } catch {
        errorRef.current = true;
        setState("error");
        setError("Malformed terminal message");
      }
    });

    socket.addEventListener("close", () => {
      if (stopped || exitedRef.current || errorRef.current) return;
      if (!attachedRef.current) {
        setState("error");
        setError("Terminal connection closed");
      } else {
        setState("disconnected");
      }
    });
    socket.addEventListener("error", () => {
      if (stopped) return;
      errorRef.current = true;
      setState("error");
      setError("Terminal connection failed");
    });

    return () => {
      stopped = true;
      attachedRef.current = false;
      if (resizeTimerRef.current) {
        clearTimeout(resizeTimerRef.current);
      }
      if (resizeSendTimerRef.current) {
        clearTimeout(resizeSendTimerRef.current);
      }
      fitTerminalRef.current = null;
      observer?.disconnect();
      dataDisposable.dispose();
      binaryDisposable.dispose();
      selectionDisposable.dispose();
      osc52Disposable.dispose();
      container.removeEventListener("mouseup", copySelectionAfterMouseUp);
      socket.close();
      socketRef.current = null;
      terminal.dispose();
    };
  }, [clientID, initialTerminalID, theme, workspaceID]);

  return (
    <div className="relative h-full min-h-0 w-full">
      <div ref={containerRef} className="h-full min-h-0 w-full p-2" data-testid="terminal-pane" />
      {state !== "connected" && state !== "exited" ? (
        <div className="pointer-events-none absolute right-3 top-3 rounded border border-border bg-background/95 px-2 py-1 text-xs text-muted-foreground shadow-sm">
          {state === "connecting" ? "Connecting" : error ?? state}
        </div>
      ) : null}
      {(state === "disconnected" || state === "error") && initialTerminalID ? (
        <div className="absolute inset-x-0 bottom-3 flex justify-center">
          <Button size="sm" variant="secondary" className="gap-2 shadow" onClick={onReconnect}>
            <RotateCcw className="h-4 w-4" />
            Reconnect
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function terminalTheme(theme: ThemeMode) {
  if (theme === "light") {
    return {
      background: "#f8fafc",
      foreground: "#0f172a",
      cursor: "#0f172a",
      selectionBackground: "#cbd5e1",
      black: "#0f172a",
      red: "#dc2626",
      green: "#15803d",
      yellow: "#a16207",
      blue: "#2563eb",
      magenta: "#9333ea",
      cyan: "#0891b2",
      white: "#e2e8f0",
    };
  }
  return {
    background: "#111315",
    foreground: "#e5e7eb",
    cursor: "#f8fafc",
    selectionBackground: "#374151",
    black: "#111827",
    red: "#f87171",
    green: "#4ade80",
    yellow: "#facc15",
    blue: "#60a5fa",
    magenta: "#c084fc",
    cyan: "#22d3ee",
    white: "#f9fafb",
  };
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunkSize = 0x8000;
  for (let index = 0; index < bytes.length; index += chunkSize) {
    const chunk = bytes.subarray(index, index + chunkSize);
    binary += String.fromCharCode(...chunk);
  }
  return btoa(binary);
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function binaryStringToBytes(value: string): Uint8Array {
  const bytes = new Uint8Array(value.length);
  for (let index = 0; index < value.length; index += 1) {
    bytes[index] = value.charCodeAt(index) & 0xff;
  }
  return bytes;
}

function terminalOsc52ClipboardText(data: string): string | null {
  const separator = data.indexOf(";");
  if (separator < 0) return null;
  const payload = data.slice(separator + 1).replace(/\s/g, "");
  if (payload === "" || payload === "?") return null;
  try {
    const bytes = base64ToBytes(payload);
    if (bytes.byteLength > maxTerminalClipboardBytes) return null;
    return new TextDecoder().decode(bytes);
  } catch {
    return null;
  }
}

async function copyTerminalText(text: string): Promise<void> {
  if (!text) return;
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through to execCommand for browsers that gate async clipboard writes.
    }
  }
  copyTerminalTextWithExecCommand(text);
}

function copyTerminalTextWithExecCommand(text: string): void {
  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "-9999px";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    if (!document.execCommand("copy")) {
      throw new Error("Copy terminal selection failed");
    }
  } finally {
    textarea.remove();
    activeElement?.focus({ preventScroll: true });
  }
}

function terminalClientID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}
