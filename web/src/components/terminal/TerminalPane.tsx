import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { RotateCcw } from "lucide-react";
import { workspaceTerminalWebSocketURL } from "@/api/client";
import type { TerminalFrame, TerminalSessionSummary } from "@/api/types";
import type { ThemeMode } from "@/theme";
import { Button } from "@/components/ui/button";
import type { TerminalConnectionState, TerminalFitMode } from "./types";
import { base64ToBytes, binaryStringToBytes, bytesToBase64, copyTerminalText, terminalOsc52ClipboardText } from "./clipboard";
import { terminalTheme } from "./theme";

export function TerminalPane({
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
