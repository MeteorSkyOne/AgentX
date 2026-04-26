import { useState } from "react";
import { Check, Hash, ChevronDown, ChevronRight, Pencil, Plus, Rows3, Trash2, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Channel } from "../api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

interface ChannelListProps {
  channels: Channel[];
  selectedChannelID?: string;
  onSelect: (channel: Channel) => void;
  onCreate: () => void;
  onUpdate: (channelID: string, name: string) => Promise<Channel>;
  onDelete: (channel: Channel) => Promise<void>;
}

export function ChannelList({ channels, selectedChannelID, onSelect, onCreate, onUpdate, onDelete }: ChannelListProps) {
  const [open, setOpen] = useState(true);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [draftName, setDraftName] = useState("");
  const [pendingID, setPendingID] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  function beginEdit(channel: Channel) {
    setEditingID(channel.id);
    setDraftName(channel.name);
    setError(null);
  }

  async function save(channel: Channel) {
    const name = draftName.trim();
    if (!name) return;
    setPendingID(channel.id);
    setError(null);
    try {
      await onUpdate(channel.id, name);
      setEditingID(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update channel failed");
    } finally {
      setPendingID(null);
    }
  }

  async function remove(channel: Channel) {
    if (!window.confirm(`Delete #${channel.name}?`)) return;
    setPendingID(channel.id);
    setError(null);
    try {
      await onDelete(channel);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete channel failed");
    } finally {
      setPendingID(null);
    }
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button className="flex w-full items-center gap-1 px-1 py-1 text-xs font-semibold uppercase text-muted-foreground hover:text-foreground">
          {open ? (
            <ChevronDown className="h-3 w-3" />
          ) : (
            <ChevronRight className="h-3 w-3" />
          )}
          Channels
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-0.5">
        {channels.map((channel) => (
          <div
            key={channel.id}
            className={cn(
              "group flex min-h-8 w-full items-center gap-1 rounded-md px-1 py-0.5 text-sm transition-colors",
              channel.id === selectedChannelID
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            )}
          >
            {editingID === channel.id ? (
              <>
                <Input
                  value={draftName}
                  onChange={(e) => setDraftName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void save(channel);
                    if (e.key === "Escape") setEditingID(null);
                  }}
                  aria-label="Channel name"
                  className="h-7 flex-1 px-2 text-sm"
                  autoFocus
                />
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  title="Save channel"
                  aria-label="Save channel"
                  disabled={pendingID === channel.id || !draftName.trim()}
                  onClick={() => save(channel)}
                >
                  <Check className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  title="Cancel"
                  aria-label="Cancel"
                  disabled={pendingID === channel.id}
                  onClick={() => setEditingID(null)}
                >
                  <X className="h-4 w-4" />
                </Button>
              </>
            ) : (
              <>
                <button
                  className="flex min-w-0 flex-1 items-center gap-2 rounded px-1 py-1 text-left"
                  onClick={() => onSelect(channel)}
                >
                  {channel.type === "thread" ? (
                    <Rows3 className="h-4 w-4 shrink-0" />
                  ) : (
                    <Hash className="h-4 w-4 shrink-0" />
                  )}
                  <span className="truncate">{channel.name}</span>
                </button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100"
                  title="Edit channel"
                  aria-label="Edit channel"
                  disabled={pendingID === channel.id}
                  onClick={() => beginEdit(channel)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100 focus:opacity-100"
                  title="Delete channel"
                  aria-label="Delete channel"
                  disabled={pendingID === channel.id}
                  onClick={() => remove(channel)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </>
            )}
          </div>
        ))}
        {error && <p className="px-2 py-1 text-xs text-destructive">{error}</p>}
        {channels.length === 0 ? (
          <p className="px-2 py-1.5 text-sm text-muted-foreground">No channels</p>
        ) : null}
        <button
          className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          title="Create channel"
          aria-label="Create channel"
          onClick={onCreate}
        >
          <Plus className="h-4 w-4" />
          <span>Create channel</span>
        </button>
      </CollapsibleContent>
    </Collapsible>
  );
}
