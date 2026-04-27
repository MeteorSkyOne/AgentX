import { lazy, Suspense, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  Brain,
  Braces,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  MessageSquare,
  Pencil,
  Reply,
  Save,
  Trash2,
  Wrench
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { ConversationAgentContext, Message, MessageReference, ProcessItem, UserPreferences } from "../api/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { AgentAvatar, agentKindColor } from "./AgentAvatar";
import { messageMetricsParts } from "./messageMetrics";

const MarkdownRenderer = lazy(() =>
  import("./MarkdownRenderer").then((module) => ({ default: module.MarkdownRenderer }))
);

interface StreamingMessage {
  runID: string;
  agentID?: string;
  text: string;
  thinking?: string;
  process?: ProcessItem[];
  error?: string;
}

interface MessagePaneProps {
  messages: Message[];
  isLoading: boolean;
  isLoadingOlder: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  agents: ConversationAgentContext[];
  preferences: UserPreferences;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onReplyMessage: (message: Message) => void;
  onLoadOlder: () => boolean;
}

type DisplayProcessItem = ProcessItem & {
  result?: ProcessItem;
};

export function MessagePane({
  messages,
  isLoading,
  isLoadingOlder,
  hasOlderMessages,
  streaming,
  agents,
  preferences,
  onUpdateMessage,
  onDeleteMessage,
  onReplyMessage,
  onLoadOlder,
}: MessagePaneProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const olderAnchorMessageIDRef = useRef<string | null>(null);
  const agentByBotID = new Map(agents.map((item) => [item.agent.bot_user_id, item.agent]));
  const agentByID = new Map(agents.map((item) => [item.agent.id, item.agent]));
  const messagesByID = new Map(messages.map((message) => [message.id, message]));

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
      className="min-h-0 flex-1"
      aria-label="Messages"
      viewportRef={viewportRef}
      onViewportScroll={handleScroll}
    >
      <section className="p-3 md:p-4">
        <div className="space-y-4">
          {isLoadingOlder && (
            <div className="py-2 text-center text-xs text-muted-foreground">
              Loading older messages...
            </div>
          )}
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
                onJumpToReplyMessage={jumpToMessage}
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
              />
            );
          })}
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

function MessageMarkdown({ text }: { text: string }) {
  return (
    <Suspense fallback={<MarkdownFallback text={text} />}>
      <MarkdownRenderer text={text} />
    </Suspense>
  );
}

function MarkdownFallback({ text }: { text: string }) {
  return <p className="whitespace-pre-wrap">{text}</p>;
}

interface MessageItemProps {
  message: Message;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
  replyAgentName?: string;
  replyTargetLoaded?: boolean;
  preferences: UserPreferences;
  onUpdateMessage: (messageID: string, body: string) => Promise<Message>;
  onDeleteMessage: (message: Message) => Promise<void>;
  onReplyMessage: (message: Message) => void;
  onJumpToReplyMessage?: (messageID: string) => void;
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
  onUpdateMessage,
  onDeleteMessage,
  onReplyMessage,
  onJumpToReplyMessage,
}: MessageItemProps) {
  const [editing, setEditing] = useState(false);
  const [draftBody, setDraftBody] = useState(message.body);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isBot = message.sender_type === "bot";
  const isSystem = message.sender_type === "system";
  const label = isBot ? agentName ?? "Agent" : isSystem ? "System" : "You";
  const initial = label.charAt(0).toUpperCase();
  const process = isBot ? processFromMetadata(message.metadata) : [];
  const metricsParts = isBot ? messageMetricsParts(message.metadata?.metrics, preferences) : [];

  useEffect(() => {
    if (!editing) {
      setDraftBody(message.body);
    }
  }, [editing, message.body, message.id]);

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
      className="group flex gap-3 rounded-md px-1 py-1 hover:bg-accent/30 md:gap-4 md:px-2"
      data-message-id={message.id}
    >
      {isBot && agentID ? (
        <AgentAvatar agentID={agentID} kind={agentKind ?? "fake"} size="md" className="shrink-0" />
      ) : (
        <Avatar className="h-10 w-10 shrink-0">
          <AvatarFallback className="text-sm">{initial}</AvatarFallback>
        </Avatar>
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
        {process.length > 0 && <ProcessBlock items={process} defaultOpen={false} />}
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
            <div className="prose prose-sm max-w-none overflow-hidden break-words select-text dark:prose-invert">
              <MessageMarkdown text={message.body} />
            </div>
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
  const body = deleted ? "Referenced message deleted" : messageReferencePreview(reference.body ?? "");
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

function messageReferencePreview(body: string): string {
  const preview = body.replace(/\s+/g, " ").trim();
  return preview || "(empty)";
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
}: {
  item: StreamingMessage;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
}) {
  const isError = Boolean(item.error);
  const label = isError ? "System" : agentName ?? "Agent";
  const process = processFromStreaming(item);

  return (
    <div className={cn("group flex gap-3 rounded-md px-1 py-1 md:gap-4 md:px-2", isError && "opacity-70")}>
      {isError ? (
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
      )}

      <div className="min-w-0 flex-1 select-text space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{label}</span>
          {!isError && (
            <Badge variant="secondary" className="text-xs">
              BOT
            </Badge>
          )}
          <span className="text-xs text-muted-foreground animate-pulse">streaming...</span>
        </div>
        {process.length > 0 && <ProcessBlock items={process} />}
        <div className="prose prose-sm max-w-none overflow-hidden break-words select-text dark:prose-invert">
          <MessageMarkdown text={item.error ?? item.text} />
        </div>
      </div>
    </div>
  );
}

function ProcessBlock({ items, defaultOpen = true }: { items: ProcessItem[]; defaultOpen?: boolean }) {
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
            <ProcessRow key={processItemKey(item, index)} item={item} />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ProcessRow({ item }: { item: DisplayProcessItem }) {
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

  return <ToolProcessRow item={item} />;
}

function ToolProcessRow({ item }: { item: DisplayProcessItem }) {
  const resultItem = item.result ?? (item.type === "tool_result" ? item : undefined);
  const isResultOnly = item.type === "tool_result" && !item.result;
  const hasResult = Boolean(resultItem);
  const status = resultItem?.status ?? item.status;
  const isError = status === "error";
  const [open, setOpen] = useState(!hasResult);
  const raw = rawProcessValue(item);
  const preview = toolPreview(item);
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
          {item.result ? "Tool" : isResultOnly ? "Tool result" : "Tool call"}
        </span>
        {item.tool_name && <span className="rounded bg-muted px-1.5 py-0.5 text-muted-foreground">{item.tool_name}</span>}
        {status && (
          <span className={cn("rounded px-1.5 py-0.5", isError ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")}>
            {status}
          </span>
        )}
        {item.tool_call_id && <span className="font-mono text-[10px] text-muted-foreground">{shortID(item.tool_call_id)}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 pt-2">
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

function processItemKey(item: DisplayProcessItem, index: number): string {
  if (item.tool_call_id) {
    return `tool-${item.tool_call_id}`;
  }
  if (item.result?.tool_call_id) {
    return `tool-${item.result.tool_call_id}`;
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

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}
