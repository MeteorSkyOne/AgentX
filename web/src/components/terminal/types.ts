import type { Workspace } from "@/api/types";
import type { ThemeMode } from "@/theme";

export interface TerminalDockProps {
  workspace?: Workspace;
  theme: ThemeMode;
  className?: string;
  onClose?: () => void;
}

export type TerminalConnectionTarget = {
  key: string;
  terminalID?: string;
  clientID?: string;
};

export type TerminalConnectionState = "connecting" | "connected" | "disconnected" | "exited" | "error";
export type TerminalSplitSide = "left" | "right";

export type TerminalPaneState = {
  id: string;
  terminalIDs: string[];
  activeTerminalID: string;
  connectionTarget: TerminalConnectionTarget | null;
};

export const primaryTerminalPaneID = "terminal-pane-primary";
export const leftTerminalPaneID = "terminal-pane-left";
export const rightTerminalPaneID = "terminal-pane-right";
export type TerminalFitMode = "debounced" | "immediate";
