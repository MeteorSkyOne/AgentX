import { Hash } from "lucide-react";
import type { Channel } from "../api/types";

interface ChannelListProps {
  channels: Channel[];
  selectedChannelID?: string;
  onSelect: (channel: Channel) => void;
}

export function ChannelList({ channels, selectedChannelID, onSelect }: ChannelListProps) {
  return (
    <nav className="channel-list" aria-label="Channels">
      <div className="section-label">Channels</div>
      {channels.map((channel) => (
        <button
          key={channel.id}
          className={channel.id === selectedChannelID ? "channel selected" : "channel"}
          onClick={() => onSelect(channel)}
          type="button"
        >
          <Hash size={16} />
          <span>{channel.name}</span>
        </button>
      ))}
      {channels.length === 0 ? <p className="sidebar-empty">No channels</p> : null}
    </nav>
  );
}
