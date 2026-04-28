import type {
  Agent,
  Channel,
  ConversationAgentContext,
  ConversationContext,
  ConversationType,
  CreateThreadResponse,
  Message,
  NotificationSettings,
  Organization,
  ProcessItem,
  Project,
  Thread,
  User,
  UserPreferences,
  Workspace,
  WorkspaceTreeEntry,
} from "../../api/types";
import type { ThemeMode } from "../../theme";
import type { SocketConnectionStatus } from "../../ws/useConversationSocket";

export interface ActiveConversation {
  type: ConversationType;
  id: string;
  projectID: string;
  channelID: string;
}

export interface StreamingMessage {
  runID: string;
  agentID?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
}

export interface ComposerConversation {
  type: ConversationType;
  id: string;
  label: string;
}

export interface ShellProps {
  user: User;
  organization?: Organization;
  projects: Project[];
  project?: Project;
  projectWorkspace?: Workspace;
  channels: Channel[];
  selectedChannel?: Channel;
  activeConversation?: ActiveConversation;
  threads: Thread[];
  agents: Agent[];
  channelAgents: ConversationAgentContext[];
  conversationContext?: ConversationContext;
  contextLoading: boolean;
  messages: Message[];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  connectionStatus: SocketConnectionStatus;
  notificationSettings?: NotificationSettings;
  notificationSettingsLoading: boolean;
  preferences: UserPreferences;
  preferencesLoading: boolean;
  theme: ThemeMode;
  onSelectProject: (projectID: string) => void;
  onCreateProject: (name: string) => Promise<Project>;
  onUpdateProject: (
    projectID: string,
    payload: { name?: string; workspace_path?: string }
  ) => Promise<Project>;
  onDeleteProject: (project: Project) => Promise<void>;
  onSelectChannel: (channel: Channel) => void;
  onCreateChannel: (name: string, type: Channel["type"]) => Promise<Channel>;
  onUpdateChannel: (channelID: string, name: string) => Promise<Channel>;
  onDeleteChannel: (channel: Channel) => Promise<void>;
  onSelectThread: (thread: Thread) => void;
  onCreateThread: (title: string, body: string) => Promise<CreateThreadResponse>;
  onUpdateThread: (threadID: string, title: string) => Promise<Thread>;
  onDeleteThread: (thread: Thread) => Promise<void>;
  onSaveChannelAgents: (
    bindings: Array<{ agent_id: string; run_workspace_id?: string }>
  ) => Promise<void>;
  onCreateAgent: (payload: {
    name: string;
    description?: string;
    handle?: string;
    kind?: string;
    model?: string;
    effort?: string;
    fast_mode?: boolean;
    yolo_mode?: boolean;
    env?: Record<string, string>;
  }) => Promise<Agent>;
  onUpdateAgent: (
    agentID: string,
    payload: Partial<Pick<Agent, "name" | "description" | "handle" | "kind" | "model" | "effort" | "enabled" | "fast_mode" | "yolo_mode">> & {
      env?: Record<string, string>;
    }
  ) => Promise<void>;
  onDeleteAgent: (agentID: string) => Promise<void>;
  onUpdateNotificationSettings: (payload: {
    webhook_enabled: boolean;
    webhook_url: string;
    webhook_secret?: string;
  }) => Promise<NotificationSettings>;
  onUpdateUserPreferences: (payload: UserPreferences) => Promise<UserPreferences>;
  onTestNotificationSettings: () => Promise<void>;
  onLoadWorkspaceTree: (workspaceID: string) => Promise<WorkspaceTreeEntry>;
  onReadWorkspaceFile: (workspaceID: string, path: string) => Promise<string>;
  onWriteWorkspaceFile: (workspaceID: string, path: string, body: string) => Promise<void>;
  onDeleteWorkspaceFile: (workspaceID: string, path: string) => Promise<void>;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onLoadOlderMessages: () => boolean;
  onMessageSent: (message: Message) => void;
  onToggleTheme: () => void;
  onLogout: () => void;
}
