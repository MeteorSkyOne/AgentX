import { memo, useContext, useEffect, useRef, useState } from "react";
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Copy,
  MessageSquare,
  Pencil,
  Reply,
  Save,
  Trash2,
} from "lucide-react";
import type { ConversationAgentContext, Message, MessageReference, TeamMetadata, UserPreferences } from "@/api/types";
import type { ThemeMode } from "@/theme";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import type { MentionLabels } from "../MarkdownRenderer";
import { cn } from "@/lib/utils";
import { AgentAvatar } from "../AgentAvatar";
import { messageMetricsParts, messageWorkingLabel } from "../messageMetrics";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Textarea } from "@/components/ui/textarea";
import { MessageAttachments } from "./MessageAttachments";
import { InterleavedMessageBody, processFromMetadata } from "./ProcessTimeline";
import type { MessageRenderItem } from "./types";
import { MentionLabelsContext, displayMentionLabels, messageBodyClassName } from "./markdown";
import { copyTextToClipboard } from "./utils";
import { formatTime } from "./time";

export function groupTeamDiscussionMessages(messages: Message[]): MessageRenderItem[] {
  const items: MessageRenderItem[] = [];
  for (let index = 0; index < messages.length; index += 1) {
    const message = messages[index];
    const team = teamMetadata(message);
    if (!isTeamDiscussion(team)) {
      items.push({ type: "message", message });
      continue;
    }

    const grouped = [message];
    let nextIndex = index + 1;
    while (nextIndex < messages.length) {
      const next = messages[nextIndex];
      const nextTeam = teamMetadata(next);
      if (!isTeamDiscussion(nextTeam) || nextTeam.session_id !== team.session_id) {
        break;
      }
      grouped.push(next);
      nextIndex += 1;
    }
    items.push({ type: "team", sessionID: team.session_id, messages: grouped });
    index = nextIndex - 1;
  }
  return items;
}

function teamMetadata(message: Message): TeamMetadata | undefined {
  return message.metadata?.team;
}

function isTeamDiscussion(team: TeamMetadata | undefined): team is TeamMetadata {
  return Boolean(team?.session_id && team.phase !== "summary");
}

export function TeamDiscussionItem({
  messages,
  agentByBotID,
  messagesByID,
  preferences,
  theme,
  onUpdateMessage,
  onDeleteMessage,
  onReplyMessage,
  onJumpToReplyMessage,
  workspacePath,
  onOpenWorkspacePath,
}: {
  messages: Message[];
  agentByBotID: Map<string, ConversationAgentContext["agent"]>;
  messagesByID: Map<string, Message>;
  preferences: UserPreferences;
  theme: ThemeMode;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onReplyMessage: (message: Message) => void;
  onJumpToReplyMessage?: (messageID: string) => void;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}) {
  const [open, setOpen] = useState(false);
  const participants = Array.from(
    new Set(
      messages
        .map((message) => agentByBotID.get(message.sender_id)?.name)
        .filter((name): name is string => Boolean(name))
    )
  );
  const title =
    participants.length > 0
      ? `Team discussion · ${participants.join(", ")}`
      : "Team discussion";

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="min-w-0 border-l-2 border-border pl-2">
      <CollapsibleTrigger asChild>
        <button className="flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-2 text-left text-sm text-muted-foreground hover:bg-accent/40 hover:text-foreground">
          {open ? <ChevronDown className="h-4 w-4 shrink-0" /> : <ChevronRight className="h-4 w-4 shrink-0" />}
          <MessageSquare className="h-4 w-4 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{title}</span>
          <Badge variant="outline" className="shrink-0 text-[11px]">
            {messages.length}
          </Badge>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3 pt-2">
        {messages.map((message) => {
          const agent = agentByBotID.get(message.sender_id);
          const replyAgent =
            message.reply_to?.sender_type === "bot"
              ? agentByBotID.get(message.reply_to.sender_id ?? "")
              : undefined;
          return (
            <MessageItem
              key={message.id}
              message={message}
              agentName={agent?.name}
              agentKind={agent?.kind}
              agentID={agent?.id}
              replyAgentName={replyAgent?.name}
              replyTargetLoaded={Boolean(
                message.reply_to && messagesByID.has(message.reply_to.message_id)
              )}
              preferences={preferences}
              onUpdateMessage={onUpdateMessage}
              onDeleteMessage={onDeleteMessage}
              onReplyMessage={onReplyMessage}
              onJumpToReplyMessage={onJumpToReplyMessage}
              theme={theme}
              workspacePath={workspacePath}
              onOpenWorkspacePath={onOpenWorkspacePath}
            />
          );
        })}
      </CollapsibleContent>
    </Collapsible>
  );
}

interface MessageItemProps {
  message: Message;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
  replyAgentName?: string;
  replyTargetLoaded?: boolean;
  preferences: UserPreferences;
  theme: ThemeMode;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onReplyMessage: (message: Message) => void;
  onJumpToReplyMessage?: (messageID: string) => void;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}

export const MessageItem = memo(function MessageItem(props: MessageItemProps) {
  if (isContextSeparatorMessage(props.message)) {
    return <ContextSeparator message={props.message} />;
  }
  return <ConversationMessageItem {...props} />;
});

function ConversationMessageItem({
  message,
  agentName,
  agentKind,
  agentID,
  replyAgentName,
  replyTargetLoaded = false,
  preferences,
  theme,
  onUpdateMessage,
  onDeleteMessage,
  onReplyMessage,
  onJumpToReplyMessage,
  workspacePath,
  onOpenWorkspacePath,
}: MessageItemProps) {
  const [editing, setEditing] = useState(false);
  const [draftBody, setDraftBody] = useState(message.body);
  const [pending, setPending] = useState(false);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const copyTimerRef = useRef<number | null>(null);
  const isBot = message.sender_type === "bot";
  const isSystem = message.sender_type === "system";
  const label = isBot ? agentName ?? "Agent" : isSystem ? "System" : "You";
  const initial = label.charAt(0).toUpperCase();
  const process = isBot ? processFromMetadata(message.metadata) : [];
  const metricsParts = isBot ? messageMetricsParts(message.metadata?.metrics, preferences) : [];
  const workingLabel = isBot ? messageWorkingLabel(message.metadata?.metrics) : null;
  const footerMetricsParts = workingLabel ? [workingLabel, ...metricsParts] : metricsParts;
  const hideAvatar = preferences.hide_avatars;

  useEffect(() => {
    if (!editing) {
      setDraftBody(message.body);
    }
    setCopied(false);
  }, [editing, message.body, message.id]);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current !== null) {
        window.clearTimeout(copyTimerRef.current);
      }
    };
  }, []);

  async function copyMessage() {
    setError(null);
    try {
      await copyTextToClipboard(message.body);
      setCopied(true);
      if (copyTimerRef.current !== null) {
        window.clearTimeout(copyTimerRef.current);
      }
      copyTimerRef.current = window.setTimeout(() => setCopied(false), 1500);
    } catch (err) {
      setCopied(false);
      setError(err instanceof Error ? err.message : "Copy message failed");
    }
  }

  async function save() {
    const body = draftBody.trim();
    if (!body) return;
    setPending(true);
    setError(null);
    try {
      await onUpdateMessage(message.id, body);
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update message failed");
    } finally {
      setPending(false);
    }
  }

  async function remove() {
    if (!window.confirm("Delete this message?")) return;
    setPending(true);
    setError(null);
    try {
      await onDeleteMessage(message);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete message failed");
    } finally {
      setPending(false);
    }
  }

  return (
    <div
      className="group flex min-w-0 max-w-full gap-3 rounded-md px-1 py-1 hover:bg-accent/30 md:gap-4 md:px-2"
      data-message-id={message.id}
    >
      {!hideAvatar && (
        isBot && agentID ? (
          <AgentAvatar agentID={agentID} kind={agentKind ?? "fake"} size="md" className="shrink-0" />
        ) : (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className="text-sm">{initial}</AvatarFallback>
          </Avatar>
        )
      )}

      <div className="min-w-0 flex-1 select-text space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{label}</span>
          {isBot && (
            <Badge variant="secondary" className="text-xs">
              BOT
            </Badge>
          )}
          <span className="text-xs text-muted-foreground">
            {formatTime(message.created_at)}
          </span>
          {!editing && (
            <div className="ml-auto flex items-center gap-1 opacity-100 transition-opacity md:opacity-0 md:group-hover:opacity-100 focus-within:opacity-100">
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                title="Reply"
                aria-label="Reply"
                disabled={pending}
                onClick={() => onReplyMessage(message)}
              >
                <Reply className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                title={copied ? "Copied" : "Copy message"}
                aria-label={copied ? "Message copied" : "Copy message"}
                disabled={pending}
                onClick={copyMessage}
              >
                {copied ? (
                  <CheckCircle2 className="h-3.5 w-3.5" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                title="Edit message"
                aria-label="Edit message"
                disabled={pending}
                onClick={() => {
                  setDraftBody(message.body);
                  setError(null);
                  setEditing(true);
                }}
              >
                <Pencil className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 text-muted-foreground hover:text-destructive"
                title="Delete message"
                aria-label="Delete message"
                disabled={pending}
                onClick={remove}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          )}
        </div>
        {message.reply_to && (
          <MessageReferencePreview
            reference={message.reply_to}
            agentName={replyAgentName}
            loaded={replyTargetLoaded}
            onOpen={onJumpToReplyMessage}
          />
        )}
        {editing ? (
          <div className="space-y-2">
            <Textarea
              value={draftBody}
              onChange={(e) => setDraftBody(e.target.value)}
              aria-label="Message body"
              rows={Math.min(8, Math.max(3, draftBody.split("\n").length))}
              className="resize-y"
            />
            {error && <p className="text-sm text-destructive">{error}</p>}
            <div className="flex items-center gap-2">
              <Button size="sm" className="gap-2" onClick={save} disabled={pending || !draftBody.trim()}>
                <Save className="h-4 w-4" />
                Save
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setDraftBody(message.body);
                  setError(null);
                  setEditing(false);
                }}
                disabled={pending}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <InterleavedMessageBody
              text={message.body}
              processItems={process}
              messageID={message.id}
              workspacePath={workspacePath}
              onOpenWorkspacePath={onOpenWorkspacePath}
              className={messageBodyClassName}
            />
            <MessageAttachments attachments={message.attachments ?? []} theme={theme} />
            {footerMetricsParts.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                {footerMetricsParts.map((part, index) => (
                  <span key={part} className="flex items-center gap-1.5">
                    {index > 0 && <span className="text-muted-foreground/60">·</span>}
                    <span>{part}</span>
                  </span>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function MessageReferencePreview({
  reference,
  agentName,
  loaded,
  onOpen,
}: {
  reference: MessageReference;
  agentName?: string;
  loaded: boolean;
  onOpen?: (messageID: string) => void;
}) {
  const mentionLabels = useContext(MentionLabelsContext);
  const deleted = Boolean(reference.deleted);
  const label = deleted ? "Referenced message" : messageReferenceSenderLabel(reference, agentName);
  const body = deleted
    ? "Referenced message deleted"
    : messageReferencePreview(reference.body ?? "", reference.attachment_count ?? 0, mentionLabels);
  const className =
    "flex min-w-0 max-w-full items-center gap-2 rounded-md border border-border bg-muted/35 px-2 py-1.5 text-left text-xs text-muted-foreground";
  const content = (
    <>
      <Reply className="h-3.5 w-3.5 shrink-0" />
      <span className="shrink-0 font-medium text-foreground">{label}</span>
      <span className="min-w-0 flex-1 truncate">{body}</span>
    </>
  );

  if (!deleted && loaded && onOpen) {
    return (
      <button
        type="button"
        className={cn(className, "transition-colors hover:bg-muted/70 hover:text-foreground")}
        title="Open referenced message"
        aria-label="Open referenced message"
        onClick={() => onOpen(reference.message_id)}
      >
        {content}
      </button>
    );
  }

  return <div className={className}>{content}</div>;
}

function messageReferenceSenderLabel(reference: MessageReference, agentName?: string): string {
  if (reference.sender_type === "user") {
    return "You";
  }
  if (reference.sender_type === "system") {
    return "System";
  }
  if (reference.sender_type === "bot") {
    return agentName ?? "Agent";
  }
  return "Message";
}

function messageReferencePreview(
  body: string,
  attachmentCount = 0,
  mentionLabels?: MentionLabels
): string {
  const preview = displayMentionLabels(body, mentionLabels).replace(/\s+/g, " ").trim();
  if (preview) {
    return preview;
  }
  if (attachmentCount > 0) {
    return `${attachmentCount} ${attachmentCount === 1 ? "attachment" : "attachments"}`;
  }
  return "(empty)";
}

function ContextSeparator({ message }: { message: Message }) {
  const mentionLabels = useContext(MentionLabelsContext);
  return (
    <div className="flex items-center gap-3 py-2" data-message-id={message.id}>
      <div className="h-px flex-1 bg-border" />
      <div className="flex min-w-0 shrink items-center gap-2 rounded-full border bg-background px-3 py-1 text-xs text-muted-foreground shadow-sm">
        <span className="truncate font-medium">
          {displayMentionLabels(message.body, mentionLabels)}
        </span>
        <span className="shrink-0 text-[11px]">{formatTime(message.created_at)}</span>
      </div>
      <div className="h-px flex-1 bg-border" />
    </div>
  );
}

function isContextSeparatorMessage(message: Message): boolean {
  return (
    message.sender_type === "system" &&
    message.metadata?.command === true &&
    message.metadata?.command_name === "new" &&
    message.metadata?.separator === true
  );
}
