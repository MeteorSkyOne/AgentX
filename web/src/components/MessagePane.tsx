import { lazy, Suspense, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { PointerEvent, WheelEvent } from "react";
import {
  Brain,
  Braces,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Copy,
  Download,
  Eye,
  FileText,
  Image as ImageIcon,
  MessageSquare,
  Pencil,
  Reply,
  Save,
  Trash2,
  Wrench
} from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchAttachmentBlob, fetchMessageProcessItem } from "../api/client";
import type {
  ConversationAgentContext,
  Message,
  MessageAttachment,
  MessageProcessItemDetail,
  MessageReference,
  ProcessItem,
  TeamMetadata,
  UserPreferences
} from "../api/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import type { ThemeMode } from "@/theme";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import type { WorkspaceFileBrowserController } from "./WorkspaceFileBrowser";
import { AgentAvatar, agentKindColor } from "./AgentAvatar";
import { messageMetricsParts } from "./messageMetrics";
import type { PendingQuestion } from "./shell/types";

const MarkdownRenderer = lazy(() =>
  import("./MarkdownRenderer").then((module) => ({ default: module.MarkdownRenderer }))
);
const LazyWorkspaceFileEditor = lazy(() =>
  import("./WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceFileEditor }))
);

const messageBodyClassName =
  "prose prose-sm min-w-0 w-full max-w-full overflow-x-auto break-words select-text dark:prose-invert";

interface StreamingMessage {
  runID: string;
  agentID?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
  team?: TeamMetadata;
}

interface MessagePaneProps {
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
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}

type DisplayProcessItem = ProcessItem & {
  result?: ProcessItem;
};

type MessageRenderItem =
  | { type: "message"; message: Message }
  | { type: "team"; sessionID: string; messages: Message[] };

export function MessagePane({
  messages,
  isLoading,
  isLoadingOlder,
  hasOlderMessages,
  streaming,
  pendingQuestion,
  agents,
  preferences,
  theme,
  onUpdateMessage,
  onDeleteMessage,
  onReplyMessage,
  onLoadOlder,
  onRespondToQuestion,
  workspacePath,
  onOpenWorkspacePath,
}: MessagePaneProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const olderAnchorMessageIDRef = useRef<string | null>(null);
  const agentByBotID = new Map(agents.map((item) => [item.agent.bot_user_id, item.agent]));
  const agentByID = new Map(agents.map((item) => [item.agent.id, item.agent]));
  const messagesByID = new Map(messages.map((message) => [message.id, message]));
  const messageItems = groupTeamDiscussionMessages(messages);

  useLayoutEffect(() => {
    const anchorID = olderAnchorMessageIDRef.current;
    if (anchorID) {
      const viewport = viewportRef.current;
      const anchor = viewport?.querySelector<HTMLElement>(
        `[data-message-id="${cssEscape(anchorID)}"]`
      );
      anchor?.scrollIntoView({ block: "start" });
      if (!isLoadingOlder) {
        olderAnchorMessageIDRef.current = null;
      }
      return;
    }
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [messages, streaming, isLoadingOlder]);

  function handleScroll() {
    const viewport = viewportRef.current;
    if (
      !viewport ||
      viewport.scrollTop > 80 ||
      isLoading ||
      isLoadingOlder ||
      !hasOlderMessages ||
      messages.length === 0
    ) {
      return;
    }

    olderAnchorMessageIDRef.current = messages[0].id;
    if (!onLoadOlder()) {
      olderAnchorMessageIDRef.current = null;
    }
  }

  function jumpToMessage(messageID: string) {
    const viewport = viewportRef.current;
    const target = viewport?.querySelector<HTMLElement>(
      `[data-message-id="${cssEscape(messageID)}"]`
    );
    target?.scrollIntoView({ block: "center" });
  }

  if (isLoading) {
    return (
      <section className="flex min-h-0 flex-1 items-center justify-center">
        <span className="text-sm text-muted-foreground">Loading messages...</span>
      </section>
    );
  }

  if (messages.length === 0 && streaming.length === 0) {
    return (
      <section className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3">
        <MessageSquare className="h-12 w-12 text-muted-foreground" />
        <span className="text-sm text-muted-foreground">No messages yet</span>
      </section>
    );
  }

  return (
    <ScrollArea
      className="min-h-0 min-w-0 flex-1"
      aria-label="Messages"
      viewportRef={viewportRef}
      viewportClassName="[&>div]:!block [&>div]:!min-w-0 [&>div]:!w-full [&>div]:!max-w-full"
      onViewportScroll={handleScroll}
    >
      <section className="min-w-0 max-w-full p-3 md:p-4">
        <div className="min-w-0 max-w-full space-y-4">
          {isLoadingOlder && (
            <div className="py-2 text-center text-xs text-muted-foreground">
              Loading older messages...
            </div>
          )}
          {messageItems.map((item) => {
            if (item.type === "team") {
              return (
                <TeamDiscussionItem
                  key={`team:${item.sessionID}`}
                  messages={item.messages}
                  agentByBotID={agentByBotID}
                  messagesByID={messagesByID}
                  preferences={preferences}
                  onUpdateMessage={onUpdateMessage}
                  onDeleteMessage={onDeleteMessage}
                  onReplyMessage={onReplyMessage}
                  onJumpToReplyMessage={jumpToMessage}
                  theme={theme}
                  workspacePath={workspacePath}
                  onOpenWorkspacePath={onOpenWorkspacePath}
                />
              );
            }
            const message = item.message;
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
                onJumpToReplyMessage={jumpToMessage}
                theme={theme}
                workspacePath={workspacePath}
                onOpenWorkspacePath={onOpenWorkspacePath}
              />
            );
          })}
          {streaming.map((item) => {
            const agent = agentByID.get(item.agentID ?? "");
            return (
              <StreamingItem
                key={item.runID}
                item={item}
                agentName={agent?.name}
                agentKind={agent?.kind}
                agentID={agent?.id}
                hideAvatar={preferences.hide_avatars}
                workspacePath={workspacePath}
                onOpenWorkspacePath={onOpenWorkspacePath}
              />
            );
          })}
          {pendingQuestion && onRespondToQuestion && (
            <QuestionPrompt
              question={pendingQuestion}
              agentName={agentByID.get(pendingQuestion.agentID)?.name}
              agentKind={agentByID.get(pendingQuestion.agentID)?.kind}
              agentID={pendingQuestion.agentID}
              hideAvatar={preferences.hide_avatars}
              onSubmit={(answer) => onRespondToQuestion(pendingQuestion.questionID, answer)}
            />
          )}
          <div ref={bottomRef} />
        </div>
      </section>
    </ScrollArea>
  );
}

function cssEscape(value: string): string {
  if (typeof CSS !== "undefined" && CSS.escape) {
    return CSS.escape(value);
  }
  return value.replace(/"/g, '\\"');
}

function MessageMarkdown({
  text,
  workspacePath,
  onOpenWorkspacePath,
}: {
  text: string;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}) {
  return (
    <Suspense fallback={<MarkdownFallback text={text} />}>
      <MarkdownRenderer
        text={text}
        workspacePath={workspacePath}
        onOpenWorkspacePath={onOpenWorkspacePath}
      />
    </Suspense>
  );
}

function MarkdownFallback({ text }: { text: string }) {
  return <p className="whitespace-pre-wrap">{text}</p>;
}

async function copyTextToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall back for browsers or test environments that block async clipboard writes.
    }
  }

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
      throw new Error("Copy message failed");
    }
  } finally {
    textarea.remove();
  }
}

function groupTeamDiscussionMessages(messages: Message[]): MessageRenderItem[] {
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

function TeamDiscussionItem({
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

function MessageItem(props: MessageItemProps) {
  if (isContextSeparatorMessage(props.message)) {
    return <ContextSeparator message={props.message} />;
  }
  return <ConversationMessageItem {...props} />;
}

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
        {process.length > 0 && <ProcessBlock items={process} defaultOpen={false} messageID={message.id} />}
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
            {message.body.trim() !== "" && (
              <div className={messageBodyClassName} data-testid="message-body">
                <MessageMarkdown
                  text={message.body}
                  workspacePath={workspacePath}
                  onOpenWorkspacePath={onOpenWorkspacePath}
                />
              </div>
            )}
            <MessageAttachments attachments={message.attachments ?? []} theme={theme} />
            {metricsParts.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                {metricsParts.map((part, index) => (
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

function MessageAttachments({
  attachments,
  theme,
}: {
  attachments: MessageAttachment[];
  theme: ThemeMode;
}) {
  if (attachments.length === 0) {
    return null;
  }
  return (
    <div className="flex max-w-full flex-wrap gap-2 pt-1">
      {attachments.map((attachment) =>
        attachment.kind === "image" ? (
          <ImageAttachment key={attachment.id} attachment={attachment} />
        ) : (
          <FileAttachment key={attachment.id} attachment={attachment} theme={theme} />
        )
      )}
    </div>
  );
}

function ImageAttachment({ attachment }: { attachment: MessageAttachment }) {
  const [objectURL, setObjectURL] = useState<string | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let nextURL: string | null = null;
    setError(null);
    fetchAttachmentBlob(attachment.id)
      .then((blob) => {
        if (cancelled) return;
        nextURL = URL.createObjectURL(blob);
        setObjectURL(nextURL);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Attachment failed");
        }
      });
    return () => {
      cancelled = true;
      if (nextURL) {
        URL.revokeObjectURL(nextURL);
      }
    };
  }, [attachment.id]);

  async function download() {
    setBusy(true);
    setError(null);
    try {
      await downloadAttachment(attachment);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Download failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="group relative h-32 w-32 overflow-hidden rounded-md border border-border bg-muted/30">
        {objectURL ? (
          <button
            type="button"
            className="block h-full w-full"
            title={attachment.filename}
            aria-label={`Preview ${attachment.filename}`}
            onClick={() => setPreviewOpen(true)}
          >
            <img
              src={objectURL}
              alt={attachment.filename}
              className="h-full w-full object-cover"
            />
          </button>
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            <ImageIcon className="h-6 w-6" />
          </div>
        )}
        <Button
          type="button"
          size="icon"
          variant="secondary"
          className="absolute right-1 top-1 h-7 w-7 opacity-95"
          title="Download attachment"
          aria-label="Download attachment"
          disabled={busy}
          onClick={download}
        >
          <Download className="h-3.5 w-3.5" />
        </Button>
        <div className="absolute inset-x-0 bottom-0 bg-background/90 px-2 py-1 text-[11px]">
          <div className="truncate font-medium">{attachment.filename}</div>
          <div className="text-muted-foreground">{formatAttachmentBytes(attachment.size_bytes)}</div>
        </div>
        {error && (
          <div className="absolute inset-x-0 top-9 px-2 text-[11px] text-destructive">
            {error}
          </div>
        )}
      </div>
      <ImageAttachmentPreviewDialog
        attachment={attachment}
        objectURL={objectURL}
        open={previewOpen}
        onOpenChange={setPreviewOpen}
      />
    </>
  );
}

function ImageAttachmentPreviewDialog({
  attachment,
  objectURL,
  open,
  onOpenChange,
}: {
  attachment: MessageAttachment;
  objectURL: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const label = imageAttachmentPreviewDialogLabel(attachment);
  const [scale, setScale] = useState(1);
  const [pan, setPan] = useState<ImagePreviewPan>({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const dragRef = useRef<{
    pointerID: number;
    startX: number;
    startY: number;
    origin: ImagePreviewPan;
  } | null>(null);

  useEffect(() => {
    if (open) {
      setScale(1);
      setPan({ x: 0, y: 0 });
      setDragging(false);
      dragRef.current = null;
    }
  }, [attachment.id, open]);

  function zoomWithWheel(event: WheelEvent<HTMLDivElement>) {
    event.preventDefault();
    setScale((current) => {
      const nextScale = nextImagePreviewScale(current, event.deltaY);
      if (nextScale <= 1) {
        setPan({ x: 0, y: 0 });
      }
      return nextScale;
    });
  }

  function startDrag(event: PointerEvent<HTMLDivElement>) {
    if (!objectURL || scale <= 1 || event.button !== 0) {
      return;
    }
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = {
      pointerID: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      origin: pan,
    };
    setDragging(true);
  }

  function dragImage(event: PointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    event.preventDefault();
    setPan(nextImagePreviewPan(drag.origin, event.clientX - drag.startX, event.clientY - drag.startY, scale));
  }

  function stopDrag(event: PointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    dragRef.current = null;
    setDragging(false);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(88vh,56rem)] max-w-[calc(100vw-2rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-6xl">
        <DialogHeader className="shrink-0 border-b border-border px-4 py-3 pr-12">
          <DialogTitle className="truncate text-sm">{label.title}</DialogTitle>
          <DialogDescription>{label.description}</DialogDescription>
        </DialogHeader>
        <div
          className={cn(
            "flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-muted/20 p-3",
            scale > 1 ? (dragging ? "cursor-grabbing" : "cursor-grab") : "cursor-zoom-in"
          )}
          data-testid="image-preview-canvas"
          onWheel={zoomWithWheel}
          onPointerDown={startDrag}
          onPointerMove={dragImage}
          onPointerUp={stopDrag}
          onPointerCancel={stopDrag}
          style={{ touchAction: "none" }}
        >
          {objectURL ? (
            <img
              src={objectURL}
              alt={attachment.filename}
              className="max-h-full max-w-full object-contain transition-transform duration-75 ease-out"
              data-testid="image-preview-image"
              draggable={false}
              style={{
                transform: `translate(${pan.x}px, ${pan.y}px) scale(${scale})`,
              }}
            />
          ) : (
            <ImageIcon className="h-8 w-8 text-muted-foreground" />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function imageAttachmentPreviewDialogLabel(
  attachment: Pick<MessageAttachment, "filename" | "content_type" | "size_bytes">
): { title: string; description: string } {
  return {
    title: attachment.filename,
    description: `${attachment.content_type || "image attachment"} · ${formatAttachmentBytes(attachment.size_bytes)}`,
  };
}

export interface ImagePreviewPan {
  x: number;
  y: number;
}

export function nextImagePreviewScale(currentScale: number, deltaY: number): number {
  const minScale = 0.25;
  const maxScale = 6;
  const multiplier = Math.exp(-deltaY * 0.0015);
  const nextScale = currentScale * multiplier;
  return Math.min(maxScale, Math.max(minScale, Number(nextScale.toFixed(3))));
}

export function nextImagePreviewPan(
  origin: ImagePreviewPan,
  deltaX: number,
  deltaY: number,
  scale: number
): ImagePreviewPan {
  if (scale <= 1) {
    return { x: 0, y: 0 };
  }
  return {
    x: Number((origin.x + deltaX).toFixed(3)),
    y: Number((origin.y + deltaY).toFixed(3)),
  };
}

function FileAttachment({
  attachment,
  theme,
}: {
  attachment: MessageAttachment;
  theme: ThemeMode;
}) {
  const [busy, setBusy] = useState<"preview" | "download" | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewBody, setPreviewBody] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function preview() {
    setBusy("preview");
    setError(null);
    try {
      const blob = await fetchAttachmentBlob(attachment.id);
      if (isTextAttachmentPreviewSupported(attachment, blob)) {
        setPreviewBody(await blob.text());
        setPreviewOpen(true);
      } else {
        openBlob(blob);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Preview failed");
    } finally {
      setBusy(null);
    }
  }

  async function download() {
    setBusy("download");
    setError(null);
    try {
      await downloadAttachment(attachment);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Download failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <div className="flex min-h-12 min-w-0 max-w-full items-center gap-2 rounded-md border border-border bg-muted/30 px-2 py-1.5 text-xs">
        <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium">{attachment.filename}</div>
          <div className="text-muted-foreground">{formatAttachmentBytes(attachment.size_bytes)}</div>
          {error && <div className="text-destructive">{error}</div>}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          title="Preview attachment"
          aria-label="Preview attachment"
          disabled={busy !== null}
          onClick={preview}
        >
          <Eye className="h-3.5 w-3.5" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          title="Download attachment"
          aria-label="Download attachment"
          disabled={busy !== null}
          onClick={download}
        >
          <Download className="h-3.5 w-3.5" />
        </Button>
      </div>
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="flex h-[min(80vh,44rem)] max-w-[calc(100vw-2rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl">
          <DialogHeader className="shrink-0 border-b border-border px-4 py-3 pr-12">
            <DialogTitle className="truncate text-sm">{attachment.filename}</DialogTitle>
            <DialogDescription>
              {attachment.content_type || "text attachment"} · {formatAttachmentBytes(attachment.size_bytes)}
            </DialogDescription>
          </DialogHeader>
          <AttachmentReadOnlyEditor
            attachment={attachment}
            body={previewBody}
            theme={theme}
          />
        </DialogContent>
      </Dialog>
    </>
  );
}

function AttachmentReadOnlyEditor({
  attachment,
  body,
  theme,
}: {
  attachment: MessageAttachment;
  body: string;
  theme: ThemeMode;
}) {
  const controller = createReadOnlyAttachmentEditorController(attachment, body);
  return (
    <Suspense
      fallback={
        <div
          className="flex min-h-0 flex-1 items-center justify-center bg-background text-xs text-muted-foreground"
          data-testid="workspace-file-editor"
          role="region"
          aria-label="Attachment preview editor"
        >
          Loading editor...
        </div>
      }
    >
      <LazyWorkspaceFileEditor
        controller={controller}
        theme={theme}
        contentAriaLabel={`Attachment preview for ${attachment.filename}`}
        className="min-h-0 flex-1"
      />
    </Suspense>
  );
}

export function createReadOnlyAttachmentEditorController(
  attachment: Pick<MessageAttachment, "filename">,
  body: string
): WorkspaceFileBrowserController {
  const noop = () => undefined;
  const asyncNoop = async () => undefined;
  return {
    workspaceID: "attachment-preview",
    workspacePath: "Attachment",
    filePath: attachment.filename,
    fileBody: body,
    tree: undefined,
    workspaceTreeResetKey: 0,
    workspaceTreeLoading: false,
    workspaceTreeError: null,
    directoryLoadingPaths: new Set(),
    directoryLoadErrors: {},
    fileLoading: false,
    fileLoadError: null,
    fileSaving: false,
    fileDeleting: false,
    entryActionPending: false,
    workspaceStatus: null,
    fileOpenPosition: undefined,
    fileOpenRequestID: 0,
    fileViewMode: "edit",
    trimmedPath: attachment.filename.trim(),
    canUseWorkspace: false,
    setFilePath: noop,
    setFileBody: noop,
    setFileViewMode: noop,
    loadTree: asyncNoop,
    loadDirectory: asyncNoop,
    loadFile: asyncNoop,
    saveFile: asyncNoop,
    deleteFile: asyncNoop,
    createEntry: async () => null,
    renameEntry: async () => null,
    deleteEntry: asyncNoop,
    moveEntry: async () => null,
  };
}

export function isTextAttachmentPreviewSupported(
  attachment: Pick<MessageAttachment, "kind" | "content_type">,
  blob?: Blob
): boolean {
  if (attachment.kind === "text") {
    return true;
  }
  const contentType = (attachment.content_type || blob?.type || "").toLowerCase();
  return contentType.startsWith("text/") || contentType.includes("json");
}

async function downloadAttachment(attachment: MessageAttachment) {
  const blob = await fetchAttachmentBlob(attachment.id);
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = attachment.filename || "attachment";
  link.style.display = "none";
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function openBlob(blob: Blob) {
  const url = URL.createObjectURL(blob);
  openObjectURL(url);
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

function openObjectURL(url: string) {
  window.open(url, "_blank", "noopener,noreferrer");
}

function formatAttachmentBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${Math.ceil(bytes / 1024)} KB`;
  }
  return `${bytes} B`;
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
  const deleted = Boolean(reference.deleted);
  const label = deleted ? "Referenced message" : messageReferenceSenderLabel(reference, agentName);
  const body = deleted
    ? "Referenced message deleted"
    : messageReferencePreview(reference.body ?? "", reference.attachment_count ?? 0);
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

function messageReferencePreview(body: string, attachmentCount = 0): string {
  const preview = body.replace(/\s+/g, " ").trim();
  if (preview) {
    return preview;
  }
  if (attachmentCount > 0) {
    return `${attachmentCount} ${attachmentCount === 1 ? "attachment" : "attachments"}`;
  }
  return "(empty)";
}

function ContextSeparator({ message }: { message: Message }) {
  return (
    <div className="flex items-center gap-3 py-2" data-message-id={message.id}>
      <div className="h-px flex-1 bg-border" />
      <div className="flex min-w-0 shrink items-center gap-2 rounded-full border bg-background px-3 py-1 text-xs text-muted-foreground shadow-sm">
        <span className="truncate font-medium">{message.body}</span>
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

function StreamingItem({
  item,
  agentName,
  agentKind,
  agentID,
  hideAvatar,
  workspacePath,
  onOpenWorkspacePath,
}: {
  item: StreamingMessage;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
  hideAvatar?: boolean;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}) {
  const isError = Boolean(item.error);
  const label = isError ? "System" : agentName ?? "Agent";
  const process = processFromStreaming(item);

  return (
    <div className={cn("group flex min-w-0 max-w-full gap-3 rounded-md px-1 py-1 md:gap-4 md:px-2", isError && "opacity-70")}>
      {!hideAvatar && (
        isError ? (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className="text-white text-sm bg-destructive">
              <CircleAlert className="h-5 w-5" />
            </AvatarFallback>
          </Avatar>
        ) : agentID ? (
          <AgentAvatar agentID={agentID} kind={agentKind ?? "fake"} size="md" className="shrink-0" />
        ) : (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className={cn("text-white text-sm", agentKindColor(agentKind ?? "fake"))}>
              <CircleAlert className="h-5 w-5" />
            </AvatarFallback>
          </Avatar>
        )
      )}

      <div className="min-w-0 flex-1 select-text space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{label}</span>
          {!isError && (
            <Badge variant="secondary" className="text-xs">
              BOT
            </Badge>
          )}
          {item.team && (
            <Badge variant="outline" className="text-xs">
              TEAM
            </Badge>
          )}
          <span className="text-xs text-muted-foreground animate-pulse">streaming...</span>
        </div>
        {process.length > 0 && <ProcessBlock items={process} />}
        <div className={messageBodyClassName} data-testid="message-body">
          <MessageMarkdown
            text={item.error ?? item.text}
            workspacePath={workspacePath}
            onOpenWorkspacePath={onOpenWorkspacePath}
          />
        </div>
      </div>
    </div>
  );
}

function ProcessBlock({
  items,
  defaultOpen = true,
  messageID,
}: {
  items: ProcessItem[];
  defaultOpen?: boolean;
  messageID?: string;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const displayItems = mergeToolProcessItems(items);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground py-1">
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        <Brain className="h-3 w-3" />
        <span>Process</span>
        <span className="text-[10px] text-muted-foreground/70">{displayItems.length}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="space-y-2 border-l border-border/60 pl-3 py-1">
          {displayItems.map((item, index) => (
            <ProcessRow key={processItemKey(item, index)} item={item} messageID={messageID} />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ProcessRow({ item, messageID }: { item: DisplayProcessItem; messageID?: string }) {
  if (item.type === "thinking") {
    return (
      <div className="space-y-1 text-xs text-muted-foreground">
        <div className="flex items-center gap-1.5 font-medium not-italic">
          <Brain className="h-3 w-3" />
          <span>Thinking</span>
        </div>
        {item.text && (
          <div className="rounded-md bg-muted/30 px-3 py-2 italic">
            <MessageMarkdown text={item.text} />
          </div>
        )}
      </div>
    );
  }

  return <ToolProcessRow item={item} messageID={messageID} />;
}

function ToolProcessRow({ item, messageID }: { item: DisplayProcessItem; messageID?: string }) {
  const [detail, setDetail] = useState<DisplayProcessItem | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const canLazyLoad =
    Boolean(messageID) && item.has_detail === true && typeof item.process_index === "number";
  const activeItem = detail ?? item;
  const resultItem = activeItem.result ?? (activeItem.type === "tool_result" ? activeItem : undefined);
  const isResultOnly = activeItem.type === "tool_result" && !activeItem.result;
  const hasResult = Boolean(resultItem);
  const status = resultItem?.status ?? activeItem.status;
  const isError = status === "error";
  const [open, setOpen] = useState(!hasResult && !canLazyLoad);
  const raw = rawProcessValue(activeItem);
  const preview = toolPreview(activeItem);

  useEffect(() => {
    setDetail(null);
    setDetailError(null);
    setDetailLoading(false);
  }, [messageID, item.process_index]);

  useEffect(() => {
    if (!open || !canLazyLoad || detail || detailLoading || detailError) {
      return;
    }
    let cancelled = false;
    setDetailLoading(true);
    fetchMessageProcessItem(messageID!, item.process_index!)
      .then((loaded) => {
        if (!cancelled) {
          setDetail(displayProcessItemFromDetail(loaded));
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setDetailError(err instanceof Error ? err.message : "Tool details failed");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [canLazyLoad, detail, detailError, item.process_index, messageID, open]);

  function retryDetailLoad() {
    setDetailError(null);
    setDetail(null);
    setOpen(true);
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border border-border/60 bg-background/35 p-2.5 text-xs"
    >
      <CollapsibleTrigger className="flex w-full flex-wrap items-center gap-2 text-left">
        {open ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
        <span
          className={cn(
            "flex h-5 w-5 items-center justify-center rounded-full",
            isError
              ? "bg-destructive/10 text-destructive"
              : hasResult
                ? "bg-emerald-500/10 text-emerald-400"
                : "bg-blue-500/10 text-blue-400"
          )}
        >
          {isError ? (
            <CircleAlert className="h-3.5 w-3.5" />
          ) : hasResult ? (
            <CheckCircle2 className="h-3.5 w-3.5" />
          ) : (
            <Wrench className="h-3.5 w-3.5" />
          )}
        </span>
        <span className="font-medium text-foreground">
          {activeItem.result ? "Tool" : isResultOnly ? "Tool result" : "Tool call"}
        </span>
        {activeItem.tool_name && <span className="rounded bg-muted px-1.5 py-0.5 text-muted-foreground">{activeItem.tool_name}</span>}
        {status && (
          <span className={cn("rounded px-1.5 py-0.5", isError ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")}>
            {status}
          </span>
        )}
        {activeItem.tool_call_id && <span className="font-mono text-[10px] text-muted-foreground">{shortID(activeItem.tool_call_id)}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 pt-2">
        {detailLoading && (
          <div className="rounded-md bg-muted/30 px-3 py-2 text-[11px] text-muted-foreground">
            Loading tool details...
          </div>
        )}
        {detailError && (
          <div className="flex flex-wrap items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-[11px] text-destructive">
            <span>{detailError}</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[11px]"
              onClick={retryDetailLoad}
            >
              Retry
            </Button>
          </div>
        )}
        {preview.title && <div className="font-medium text-muted-foreground">{preview.title}</div>}
        {preview.command && (
          <ScrollArea orientation="horizontal" className="rounded-md bg-sidebar/80">
            <pre className="min-w-max px-3 py-2 font-mono text-[12px] leading-relaxed text-foreground">
              {preview.command}
            </pre>
          </ScrollArea>
        )}
        {preview.body && (
          <ScrollArea className="max-h-64 rounded-md bg-sidebar/80" viewportClassName="max-h-64">
            <pre className="whitespace-pre-wrap px-3 py-2 font-mono text-[12px] leading-relaxed text-muted-foreground">
              {preview.body}
            </pre>
          </ScrollArea>
        )}
        {preview.params.length > 0 && (
          <div className="grid gap-1.5">
            {preview.params.map(([key, value]) => (
              <div key={key} className="grid grid-cols-[6rem_1fr] gap-2 rounded bg-muted/25 px-2 py-1">
                <span className="truncate text-muted-foreground">{key}</span>
                <span className="min-w-0 truncate font-mono text-[11px] text-foreground">{value}</span>
              </div>
            ))}
          </div>
        )}
        {raw !== undefined && (
          <details className="group">
            <summary className="flex cursor-pointer list-none items-center gap-1.5 text-[11px] text-muted-foreground hover:text-foreground">
              <Braces className="h-3 w-3" />
              <span>Details</span>
            </summary>
            <ScrollArea orientation="both" className="mt-1 max-h-80 rounded-md bg-sidebar" viewportClassName="max-h-80">
              <pre className="min-w-max p-3 text-[11px] leading-relaxed text-muted-foreground">
                {formatJSON(raw)}
              </pre>
            </ScrollArea>
          </details>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

interface ToolPreview {
  title?: string;
  command?: string;
  body?: string;
  params: Array<[string, string]>;
}

function toolPreview(item: DisplayProcessItem): ToolPreview {
  const resultItem = item.result ?? (item.type === "tool_result" ? item : undefined);
  const inputValue = item.type === "tool_result" && !item.result ? item.input : item.input ?? resultItem?.input;
  const outputValue = resultItem?.output ?? resultItem?.text ?? resultItem?.raw;
  const inputRaw = item.raw;
  const resultRaw = resultItem?.raw;
  const command = extractCommand(inputValue) ?? extractCommand(inputRaw) ?? extractCommand(resultRaw);
  const cwd =
    extractString(inputValue, "cwd", "workdir", "working_directory") ??
    extractString(inputRaw, "cwd", "workdir", "working_directory") ??
    extractString(resultRaw, "cwd", "workdir", "working_directory");
  const description =
    extractString(inputValue, "description", "reason") ??
    extractString(inputRaw, "description", "reason") ??
    extractString(resultRaw, "description", "reason");
  const body = resultItem ? toolOutputText(outputValue) : undefined;
  const params = previewParams(inputValue, new Set(["command", "cmd", "cwd", "workdir", "working_directory", "description", "reason"]));

  return {
    title: description ?? (cwd ? `cwd: ${cwd}` : undefined),
    command,
    body: body && body !== command ? clampText(body, 4000) : undefined,
    params
  };
}

function extractCommand(value: unknown): string | undefined {
  const record = asRecord(value);
  if (!record) return undefined;
  const command = record.command ?? record.cmd;
  if (Array.isArray(command)) {
    return command.map((part) => shellPart(String(part))).join(" ");
  }
  if (typeof command === "string" && command.trim() !== "") {
    return command;
  }
  const nested = asRecord(record.input) ?? asRecord(record.arguments);
  if (nested) {
    return extractCommand(nested);
  }
  return undefined;
}

function extractString(value: unknown, ...keys: string[]): string | undefined {
  const record = asRecord(value);
  if (!record) return undefined;
  for (const key of keys) {
    const item = record[key];
    if (typeof item === "string" && item.trim() !== "") return item;
  }
  return undefined;
}

function previewParams(value: unknown, excluded: Set<string>): Array<[string, string]> {
  const record = asRecord(value);
  if (!record) return [];
  return Object.entries(record)
    .filter(([key]) => !excluded.has(key))
    .slice(0, 6)
    .map(([key, value]) => [key, compactValue(value)]);
}

function toolOutputText(value: unknown): string | undefined {
  if (typeof value === "string") return value;
  const record = asRecord(value);
  if (!record) return value === undefined || value === null ? undefined : compactValue(value);
  for (const key of ["output", "aggregated_output", "content", "result", "stdout", "stderr", "formatted_output"]) {
    const item = record[key];
    if (typeof item === "string" && item !== "") return item;
  }
  return formatJSON(value);
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

function compactValue(value: unknown): string {
  if (typeof value === "string") return clampText(value, 240).replace(/\s+/g, " ");
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (value == null) return "";
  return clampText(formatJSON(value), 240).replace(/\s+/g, " ");
}

function clampText(value: string, limit: number): string {
  if (value.length <= limit) return value;
  return `${value.slice(0, limit).trimEnd()}\n...`;
}

function shortID(value: string): string {
  return value.length > 18 ? `${value.slice(0, 10)}...${value.slice(-4)}` : value;
}

function shellPart(value: string): string {
  return /^[A-Za-z0-9_/:=.,@%+-]+$/.test(value) ? value : JSON.stringify(value);
}

function mergeToolProcessItems(items: ProcessItem[]): DisplayProcessItem[] {
  const merged: DisplayProcessItem[] = [];
  const toolCallIndexByID = new Map<string, number>();

  for (const item of items) {
    if (item.type === "tool_call") {
      merged.push({ ...item });
      if (item.tool_call_id) {
        toolCallIndexByID.set(item.tool_call_id, merged.length - 1);
      }
      continue;
    }

    if (item.type === "tool_result") {
      const matchingIndex = item.tool_call_id
        ? toolCallIndexByID.get(item.tool_call_id)
        : findPreviousOpenToolCallIndex(merged, item);

      if (matchingIndex !== undefined) {
        merged[matchingIndex] = mergeToolProcessPair(merged[matchingIndex], item);
        continue;
      }
    }

    merged.push({ ...item });
  }

  return merged;
}

function findPreviousOpenToolCallIndex(items: DisplayProcessItem[], result: ProcessItem): number | undefined {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const candidate = items[index];
    if (candidate.type !== "tool_call" || candidate.result) {
      continue;
    }
    if (candidate.tool_name && result.tool_name && candidate.tool_name !== result.tool_name) {
      continue;
    }
    return index;
  }
  return undefined;
}

function mergeToolProcessPair(call: DisplayProcessItem, result: ProcessItem): DisplayProcessItem {
  return {
    ...call,
    tool_name: call.tool_name ?? result.tool_name,
    tool_call_id: call.tool_call_id ?? result.tool_call_id,
    status: result.status ?? call.status,
    input: call.input ?? result.input,
    output: result.output ?? result.text ?? call.output,
    result
  };
}

function displayProcessItemFromDetail(detail: MessageProcessItemDetail): DisplayProcessItem {
  const item: DisplayProcessItem = { ...detail.item };
  if (!detail.result) {
    return item;
  }
  return mergeToolProcessPair(item, detail.result);
}

function processItemKey(item: DisplayProcessItem, index: number): string {
  if (item.tool_call_id) {
    return `tool-${item.tool_call_id}`;
  }
  if (item.result?.tool_call_id) {
    return `tool-${item.result.tool_call_id}`;
  }
  if (typeof item.process_index === "number") {
    return `${item.type}-${item.process_index}`;
  }
  return `${item.type}-${index}`;
}

function processFromMetadata(metadata: Message["metadata"]): ProcessItem[] {
  if (metadata?.process && Array.isArray(metadata.process) && metadata.process.length > 0) {
    return metadata.process;
  }
  if (typeof metadata?.thinking === "string" && metadata.thinking.trim() !== "") {
    return [{ type: "thinking", text: metadata.thinking }];
  }
  return [];
}

function processFromStreaming(item: StreamingMessage): ProcessItem[] {
  if (item.process && item.process.length > 0) {
    return item.process;
  }
  if (item.thinking && item.thinking.trim() !== "") {
    return [{ type: "thinking", text: item.thinking }];
  }
  return [];
}

function rawProcessValue(item: DisplayProcessItem): unknown {
  if (item.result) {
    return {
      call: rawSingleProcessValue(item),
      result: rawSingleProcessValue(item.result)
    };
  }
  return rawSingleProcessValue(item);
}

function rawSingleProcessValue(item: ProcessItem): unknown {
  if (item.raw !== undefined) {
    return item.raw;
  }
  if (item.input !== undefined || item.output !== undefined) {
    return {
      ...(item.input !== undefined ? { input: item.input } : {}),
      ...(item.output !== undefined ? { output: item.output } : {})
    };
  }
  return undefined;
}

function formatJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return String(value);
  }
}

function QuestionPrompt({
  question,
  agentName,
  agentKind,
  agentID,
  hideAvatar,
  onSubmit,
}: {
  question: PendingQuestion;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
  hideAvatar?: boolean;
  onSubmit: (answer: string) => void;
}) {
  const [answer, setAnswer] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const label = agentName ?? "Agent";

  async function handleSubmit() {
    if (submitting) return;
    setSubmitting(true);
    try {
      onSubmit(answer);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="group flex min-w-0 max-w-full gap-3 rounded-md px-1 py-1 md:gap-4 md:px-2">
      {!hideAvatar && (
        agentID ? (
          <AgentAvatar agentID={agentID} kind={agentKind ?? "fake"} size="md" className="shrink-0" />
        ) : (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className={cn("text-white text-sm", agentKindColor(agentKind ?? "fake"))}>?</AvatarFallback>
          </Avatar>
        )
      )}

      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{label}</span>
          <Badge variant="secondary" className="text-xs">BOT</Badge>
          <Badge variant="outline" className="text-xs text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-600">QUESTION</Badge>
        </div>
        <div className={messageBodyClassName} data-testid="question-body">
          <Suspense fallback={<p>{question.question}</p>}>
            <MarkdownRenderer text={question.question} />
          </Suspense>
        </div>
        {question.options && question.options.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {question.options.map((opt) => (
              <Button
                key={opt.label}
                variant={answer === opt.label ? "default" : "outline"}
                size="sm"
                onClick={() => setAnswer(opt.label)}
                disabled={submitting}
              >
                <span>{opt.label}</span>
                {opt.description && (
                  <span className="ml-1 text-muted-foreground text-xs">— {opt.description}</span>
                )}
              </Button>
            ))}
          </div>
        )}
        <div className="flex gap-2 items-end">
          <Textarea
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
            placeholder="Type your response..."
            className="min-h-[40px] max-h-[120px] resize-none text-sm"
            rows={1}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                handleSubmit();
              }
            }}
            disabled={submitting}
          />
          <Button
            onClick={handleSubmit}
            disabled={submitting}
            size="sm"
            className="shrink-0"
          >
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}
