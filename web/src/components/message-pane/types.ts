import type {
  ConversationAgentContext,
  Message,
  ProcessItem,
  TeamMetadata,
  UserPreferences,
} from "@/api/types";
import type { ThemeMode } from "@/theme";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import type { PendingQuestion } from "../shell/types";

export interface StreamingMessage {
  runID: string;
  agentID?: string;
  startedAt?: string;
  endedAt?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
  team?: TeamMetadata;
}

export interface MessagePaneProps {
  messages: Message[];
  isLoading: boolean;
  isLoadingOlder: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  pendingQuestion?: PendingQuestion | null;
  agents: ConversationAgentContext[];
  preferences: UserPreferences;
  theme: ThemeMode;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onReplyMessage: (message: Message) => void;
  onLoadOlder: () => boolean;
  onRespondToQuestion?: (questionID: string, answer: string) => Promise<void>;
  conversationKey?: string;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}

export type DisplayProcessItem = ProcessItem & {
  result?: ProcessItem;
};

export type DisplayProcessToolFragment = {
  type: "tool_fragment";
  items: DisplayProcessItem[];
};

export type DisplayProcessEntry = DisplayProcessItem | DisplayProcessToolFragment;

export type MessageRenderItem =
  | { type: "message"; message: Message }
  | { type: "team"; sessionID: string; messages: Message[] };
