import type { TerminalSessionSummary } from "@/api/types";
import type { TerminalConnectionTarget, TerminalPaneState, TerminalSplitSide } from "./types";
import { leftTerminalPaneID, primaryTerminalPaneID, rightTerminalPaneID } from "./types";
import { terminalClientID } from "./utils";

export function createTerminalPane(
  id: string,
  terminalIDs: string[] = [],
  activeTerminalID = "",
  connectionTarget: TerminalConnectionTarget | null = null
): TerminalPaneState {
  return { id, terminalIDs, activeTerminalID, connectionTarget };
}

export function ensurePane(panes: TerminalPaneState[], paneID: string): TerminalPaneState[] {
  if (panes.some((pane) => pane.id === paneID)) {
    return panes;
  }
  return [...panes, createTerminalPane(paneID)];
}

export function terminalConnectionTarget(terminalID: string): TerminalConnectionTarget {
  return { key: `session:${terminalID}`, terminalID };
}

export function terminalReconnectTarget(terminalID: string): TerminalConnectionTarget {
  return { key: `session:${terminalID}:reconnect:${terminalClientID()}`, terminalID };
}

export function terminalNewConnectionTarget(): TerminalConnectionTarget {
  const clientID = terminalClientID();
  return { key: `new:${clientID}`, clientID };
}

export function appendUnique(values: string[], value: string): string[] {
  return values.includes(value) ? values : [...values, value];
}

export function preferredSessionForIDs(
  sessions: TerminalSessionSummary[],
  terminalIDs: string[]
): TerminalSessionSummary | undefined {
  const paneSessions = terminalIDs
    .map((id) => sessions.find((session) => session.id === id))
    .filter((session): session is TerminalSessionSummary => Boolean(session));
  return paneSessions.find((session) => session.status === "running") ?? paneSessions[paneSessions.length - 1];
}

export function normalizePaneSelection(
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

export function ensurePaneHasContent(pane: TerminalPaneState): TerminalPaneState {
  if (pane.terminalIDs.length > 0 || pane.connectionTarget) {
    return pane;
  }
  return { ...pane, activeTerminalID: "", connectionTarget: terminalNewConnectionTarget() };
}

export function compactTerminalPanesAfterClose(
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

export function splitTerminalPanes(
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
