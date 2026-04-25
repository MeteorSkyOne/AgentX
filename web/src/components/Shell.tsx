import { LogOut, UserRound } from "lucide-react";
import { ChannelList } from "./ChannelList";
import { Composer } from "./Composer";
import { MessagePane } from "./MessagePane";
import type { Channel, Message, Organization, User } from "../api/types";

interface StreamingMessage {
  text: string;
  error?: string;
}

interface ShellProps {
  user: User;
  organization?: Organization;
  channels: Channel[];
  selectedChannel?: Channel;
  messages: Message[];
  messagesLoading: boolean;
  streaming: StreamingMessage | null;
  onSelectChannel: (channel: Channel) => void;
  onMessageSent: () => void;
  onLogout: () => void;
}

export function Shell({
  user,
  organization,
  channels,
  selectedChannel,
  messages,
  messagesLoading,
  streaming,
  onSelectChannel,
  onMessageSent,
  onLogout
}: ShellProps) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="workspace-switcher">
          <div>
            <span className="section-label">Workspace</span>
            <strong>{organization?.name ?? "AgentX"}</strong>
          </div>
        </div>
        <ChannelList
          channels={channels}
          selectedChannelID={selectedChannel?.id}
          onSelect={onSelectChannel}
        />
      </aside>

      <main className="conversation">
        <header className="topbar">
          <div className="conversation-title">
            <span className="hash-mark">#</span>
            <h1>{selectedChannel?.name ?? "No channel"}</h1>
          </div>
          <div className="topbar-actions">
            <div className="current-user">
              <UserRound size={16} />
              <span>{user.display_name}</span>
            </div>
            <button className="icon-button" type="button" title="Log out" aria-label="Log out" onClick={onLogout}>
              <LogOut size={18} />
            </button>
          </div>
        </header>
        <MessagePane messages={messages} isLoading={messagesLoading} streaming={streaming} />
        <Composer channel={selectedChannel} onSent={onMessageSent} />
      </main>
    </div>
  );
}
