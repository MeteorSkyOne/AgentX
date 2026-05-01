import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ChangeEvent, ClipboardEvent, DragEvent, FormEvent, KeyboardEvent } from "react";
import { skipToken, useQuery } from "@tanstack/react-query";
import { AlertCircle, BookOpen, FileText, Image as ImageIcon, Paperclip, Reply, Send, Terminal, X } from "lucide-react";
import { conversationSkills, sendMessage } from "../api/client";
import type { Agent, ConversationAgentSkills, ConversationSkill, ConversationType, Message } from "../api/types";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface TypingAgent {
  name: string;
}

interface DraftAttachment {
  id: string;
  file: File;
  previewURL?: string;
}

interface ComposerProps {
  conversation?: {
    type: ConversationType;
    id: string;
    label: string;
  };
  typingAgents?: TypingAgent[];
  mentionAgents?: Pick<Agent, "id" | "name" | "handle" | "kind" | "bot_user_id">[];
  replyToMessage?: Message | null;
  onCancelReplyTo?: () => void;
  onSent: (message: Message) => void;
}

export function Composer({
  conversation,
  typingAgents,
  mentionAgents = [],
  replyToMessage,
  onCancelReplyTo,
  onSent
}: ComposerProps) {
  const [body, setBody] = useState("");
  const [attachments, setAttachments] = useState<DraftAttachment[]>([]);
  const [draggingFiles, setDraggingFiles] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [caret, setCaret] = useState(0);
  const [commandIndex, setCommandIndex] = useState(0);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [dismissedCommandKey, setDismissedCommandKey] = useState<string | null>(null);
  const [dismissedMentionKey, setDismissedMentionKey] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const attachmentsRef = useRef<DraftAttachment[]>([]);
  const trimmed = body.trim();
  const commandToken = useMemo(() => slashCommandTokenAt(body, caret), [body, caret]);
  const skillsConversation = conversation && commandToken ? conversation : null;
  const skillsQuery = useQuery({
    queryKey: ["conversation-skills", skillsConversation?.type, skillsConversation?.id],
    queryFn: skillsConversation
      ? () => conversationSkills(skillsConversation.type, skillsConversation.id)
      : skipToken,
    staleTime: 15_000
  });
  const allSlashCommands = useMemo(
    () => buildSlashCommandOptions(skillsQuery.data ?? [], mentionAgents.length > 1),
    [skillsQuery.data, mentionAgents.length]
  );
  const commandIndicator = useMemo(
    () =>
      commandToken
        ? slashCommandIndicatorForName(commandToken.query, allSlashCommands)
        : slashCommandIndicator(trimmed, allSlashCommands),
    [allSlashCommands, commandToken, trimmed]
  );
  const commandKey = commandToken
    ? `${commandToken.start}:${commandToken.end}:${commandToken.query}`
    : null;
  const commandMatches = useMemo(() => {
    if (!commandToken) return [];
    const query = commandLookupKey(commandToken.query);
    return allSlashCommands
      .filter((command) => commandLookupKey(command.name).startsWith(query))
      .slice(0, 8);
  }, [allSlashCommands, commandToken]);
  const commandOpen = Boolean(
    commandToken && commandMatches.length > 0 && commandKey !== dismissedCommandKey
  );
  const mentionToken = useMemo(() => mentionTokenAt(body, caret), [body, caret]);
  const mentionKey = mentionToken
    ? `${mentionToken.start}:${mentionToken.end}:${mentionToken.query}`
    : null;
  const mentionMatches = useMemo(() => {
    if (!mentionToken) return [];
    const query = mentionToken.query.toLowerCase();
    return mentionAgents
      .filter((agent) => {
        const handle = agent.handle.toLowerCase();
        const name = agent.name.toLowerCase();
        return handle.includes(query) || name.includes(query);
      })
      .slice(0, 6);
  }, [mentionAgents, mentionToken]);
  const mentionOpen = Boolean(mentionToken && mentionMatches.length > 0 && mentionKey !== dismissedMentionKey);

  const resize = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    const next = Math.min(el.scrollHeight, 200);
    el.style.height = `${next}px`;
    el.style.overflow = el.scrollHeight > 200 ? "auto" : "hidden";
  }, []);

  useEffect(() => {
    resize();
  }, [body, resize]);

  useEffect(() => {
    setMentionIndex(0);
  }, [mentionToken?.query]);

  useEffect(() => {
    setCommandIndex(0);
  }, [commandToken?.query]);

  useEffect(() => {
    if (!commandToken) {
      setDismissedCommandKey(null);
    }
  }, [commandToken]);

  useEffect(() => {
    if (!mentionToken) {
      setDismissedMentionKey(null);
    }
  }, [mentionToken]);

  useEffect(() => {
    attachmentsRef.current = attachments;
  }, [attachments]);

  useEffect(() => {
    return () => {
      revokeAttachmentPreviews(attachmentsRef.current);
    };
  }, []);

  function addFiles(files: File[]) {
    if (files.length === 0) return;

    const selection = selectDraftAttachmentFiles(attachments.map((item) => item.file), files);
    if (selection.rejected.length > 0) {
      setError(selection.rejected.slice(0, 3).join("; "));
    } else {
      setError(null);
    }
    if (selection.accepted.length > 0) {
      setAttachments((current) => [...current, ...selection.accepted.map(createDraftAttachment)]);
    }
  }

  function removeAttachment(attachmentID: string) {
    setAttachments((current) => {
      const removed = current.find((item) => item.id === attachmentID);
      if (removed?.previewURL) {
        URL.revokeObjectURL(removed.previewURL);
      }
      return current.filter((item) => item.id !== attachmentID);
    });
  }

  function clearAttachments(items: DraftAttachment[], revokePreviews = true) {
    if (revokePreviews) {
      revokeAttachmentPreviews(items);
    }
    setAttachments([]);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }

  function handleFileInputChange(event: ChangeEvent<HTMLInputElement>) {
    addFiles(Array.from(event.currentTarget.files ?? []));
    event.currentTarget.value = "";
  }

  function handlePaste(event: ClipboardEvent<HTMLTextAreaElement>) {
    const files = Array.from(event.clipboardData.files ?? []);
    if (files.length === 0) return;
    event.preventDefault();
    addFiles(files);
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>) {
    if (!hasDraggedFiles(event)) return;
    event.preventDefault();
    setDraggingFiles(true);
  }

  function handleDragLeave(event: DragEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setDraggingFiles(false);
    }
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    if (!hasDraggedFiles(event)) return;
    event.preventDefault();
    setDraggingFiles(false);
    addFiles(Array.from(event.dataTransfer.files ?? []));
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!conversation || (trimmed === "" && attachments.length === 0)) {
      return;
    }

    const submittedBody = trimmed;
    const submittedAttachments = attachments;
    setError(null);
    setSubmitting(true);
    setBody("");
    clearAttachments(submittedAttachments, false);
    setCaret(0);
    setCommandIndex(0);
    setMentionIndex(0);
    setDismissedCommandKey(null);
    setDismissedMentionKey(null);
    try {
      const message = await sendMessage(conversation.type, conversation.id, submittedBody, {
        replyToMessageID: replyToMessage?.id,
        files: submittedAttachments.map((attachment) => attachment.file)
      });
      onSent(message);
      revokeAttachmentPreviews(submittedAttachments);
    } catch (err) {
      setBody(submittedBody);
      setCaret(submittedBody.length);
      setAttachments(submittedAttachments);
      setError(err instanceof Error ? err.message : "Message failed");
    } finally {
      setSubmitting(false);
    }
  }

  function insertMention(agent: Pick<Agent, "handle">) {
    if (!mentionToken) return;
    const next = `${body.slice(0, mentionToken.start)}@${agent.handle} ${body.slice(mentionToken.end)}`;
    const nextCaret = mentionToken.start + agent.handle.length + 2;
    setBody(next);
    setCaret(nextCaret);
    setDismissedMentionKey(null);
    requestAnimationFrame(() => {
      textareaRef.current?.focus();
      textareaRef.current?.setSelectionRange(nextCaret, nextCaret);
    });
  }

  function insertCommand(command: SlashCommandDefinition) {
    if (!commandToken) return;
    const beforeToken = body.slice(0, commandToken.start).trim();
    const afterToken = body.slice(commandToken.end).trim();
    const args = [beforeToken, afterToken].filter(Boolean).join(" ");
    const prefix =
      command.kind === "skill" && command.agentHandle && mentionAgents.length > 1
        ? `/${command.name} @${command.agentHandle}`
        : `/${command.name}`;
    const next = args ? `${prefix} ${args}` : `${prefix} `;
    const nextCaret = next.length;
    setBody(next);
    setCaret(nextCaret);
    setDismissedCommandKey(null);
    requestAnimationFrame(() => {
      textareaRef.current?.focus();
      textareaRef.current?.setSelectionRange(nextCaret, nextCaret);
    });
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (commandOpen) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setCommandIndex((index) => (index + 1) % commandMatches.length);
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setCommandIndex((index) => (index - 1 + commandMatches.length) % commandMatches.length);
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        insertCommand(commandMatches[commandIndex]);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setDismissedCommandKey(commandKey);
        return;
      }
    }
    if (mentionOpen) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setMentionIndex((index) => (index + 1) % mentionMatches.length);
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setMentionIndex((index) => (index - 1 + mentionMatches.length) % mentionMatches.length);
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        insertMention(mentionMatches[mentionIndex]);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setDismissedMentionKey(mentionKey);
        return;
      }
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  }

  const typingLabel =
    typingAgents && typingAgents.length > 0
      ? typingAgents.length === 1
        ? `${typingAgents[0].name} is typing`
        : `${typingAgents.map((a) => a.name).join(", ")} are typing`
      : null;

  return (
    <div
      className="shrink-0 border-t border-border bg-background/95 px-3 pt-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] md:px-4"
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {typingLabel && (
        <div className="flex items-center gap-2 px-1 py-1.5 text-xs text-muted-foreground">
          <span className="flex gap-0.5">
            <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground animate-bounce [animation-delay:0ms]" />
            <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground animate-bounce [animation-delay:150ms]" />
            <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground animate-bounce [animation-delay:300ms]" />
          </span>
          <span>{typingLabel}</span>
        </div>
      )}
      <form onSubmit={handleSubmit}>
        <div className="relative">
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            accept={draftAttachmentAccept}
            onChange={handleFileInputChange}
          />
          {commandOpen && (
            <div className="absolute bottom-full left-0 mb-2 w-[min(22rem,calc(100vw-1.5rem))] overflow-hidden rounded-md border border-border bg-popover shadow-lg">
              {commandMatches.map((command, index) => (
                <button
                  key={slashCommandKey(command)}
                  type="button"
                  className={cn(
                    "flex w-full items-center gap-3 px-3 py-2 text-left text-sm",
                    index === commandIndex
                      ? "bg-accent text-accent-foreground"
                      : "hover:bg-accent/60"
                  )}
                  onMouseDown={(event) => {
                    event.preventDefault();
                    insertCommand(command);
                  }}
                >
                  {command.kind === "skill" ? (
                    <BookOpen className="h-4 w-4 shrink-0 text-muted-foreground" />
                  ) : (
                    <Terminal className="h-4 w-4 shrink-0 text-muted-foreground" />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="truncate font-medium">/{command.name}</span>
                      {command.kind === "skill" && command.agentHandle && mentionAgents.length > 1 ? (
                        <span className="shrink-0 rounded border border-border bg-background/70 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                          @{command.agentHandle}
                        </span>
                      ) : null}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {command.description}
                    </span>
                  </span>
                </button>
              ))}
            </div>
          )}
          {mentionOpen && (
            <div className="absolute bottom-full left-0 mb-2 w-[min(18rem,calc(100vw-1.5rem))] overflow-hidden rounded-md border border-border bg-popover shadow-lg">
              {mentionMatches.map((agent, index) => (
                <button
                  key={agent.id}
                  type="button"
                  className={`flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm ${
                    index === mentionIndex ? "bg-accent text-accent-foreground" : "hover:bg-accent/60"
                  }`}
                  onMouseDown={(event) => {
                    event.preventDefault();
                    insertMention(agent);
                  }}
                >
                  <span className="truncate font-medium">{agent.name}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">@{agent.handle}</span>
                </button>
              ))}
            </div>
          )}
          {replyToMessage && (
            <div className="mb-2 flex min-w-0 items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1.5 text-xs">
              <Reply className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <span className="shrink-0 font-medium text-foreground">
                Replying to {messageSenderLabel(replyToMessage, mentionAgents)}
              </span>
              <span className="min-w-0 flex-1 truncate text-muted-foreground">
                {messagePreview(replyToMessage.body)}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-6 w-6 shrink-0"
                title="Cancel reply"
                aria-label="Cancel reply"
                onClick={onCancelReplyTo}
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          )}
          {attachments.length > 0 && (
            <div className="mb-2 flex max-h-28 flex-wrap gap-2 overflow-y-auto">
              {attachments.map((attachment) => (
                <div
                  key={attachment.id}
                  className="flex h-10 max-w-full items-center gap-2 rounded-md border border-border bg-muted/35 px-2 text-xs"
                >
                  {attachment.previewURL ? (
                    <img
                      src={attachment.previewURL}
                      alt={attachment.file.name || "attachment"}
                      className="h-7 w-7 rounded object-cover"
                    />
                  ) : isImageFile(attachment.file) ? (
                    <ImageIcon className="h-4 w-4 shrink-0 text-muted-foreground" />
                  ) : (
                    <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
                  )}
                  <span className="min-w-0 max-w-48 truncate font-medium">
                    {attachment.file.name || "attachment"}
                  </span>
                  <span className="shrink-0 text-muted-foreground">
                    {formatBytes(attachment.file.size)}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6 shrink-0"
                    title="Remove attachment"
                    aria-label="Remove attachment"
                    disabled={submitting}
                    onClick={() => removeAttachment(attachment.id)}
                  >
                    <X className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          <div
            className={cn(
              "flex min-h-11 items-center gap-2 rounded-md border border-input bg-secondary/60 px-3 py-2 shadow-xs transition-[background-color,border-color,box-shadow] focus-within:border-ring focus-within:bg-background focus-within:ring-[3px] focus-within:ring-ring/20",
              draggingFiles && "border-primary/70 bg-primary/5 ring-[3px] ring-primary/20",
              commandIndicator?.status === "recognized" &&
                "border-primary/60 bg-primary/5 focus-within:border-primary/70 focus-within:ring-primary/20",
              commandIndicator?.status === "pending" &&
                "border-ring/50 bg-accent/40 focus-within:border-ring",
              commandIndicator?.status === "unknown" &&
                "border-destructive/60 bg-destructive/5 focus-within:border-destructive/70 focus-within:ring-destructive/20"
            )}
          >
            {commandIndicator ? (
              <span
                className={cn(
                  "flex h-7 max-w-[9rem] shrink-0 items-center gap-1 rounded-md border px-2 text-xs font-medium",
                  commandIndicator.status === "recognized" &&
                    "border-primary/30 bg-primary/10 text-primary",
                  commandIndicator.status === "pending" &&
                    "border-border bg-background/80 text-muted-foreground",
                  commandIndicator.status === "unknown" &&
                    "border-destructive/30 bg-destructive/10 text-destructive"
                )}
                title={commandIndicator.title}
                aria-label={commandIndicator.title}
              >
                {commandIndicator.status === "unknown" ? (
                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                ) : (
                  <Terminal className="h-3.5 w-3.5 shrink-0" />
                )}
                <span className="truncate">{commandIndicator.label}</span>
              </span>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0 self-end"
              title="Attach files"
              aria-label="Attach files"
              disabled={!conversation || submitting}
              onClick={() => fileInputRef.current?.click()}
            >
              <Paperclip className="h-4 w-4" />
            </Button>
            <textarea
              ref={textareaRef}
              value={body}
              onChange={(event) => {
                setBody(event.target.value);
                setCaret(event.target.selectionStart);
              }}
              onClick={(event) => setCaret(event.currentTarget.selectionStart)}
              onKeyDown={handleKeyDown}
              onKeyUp={(event) => setCaret(event.currentTarget.selectionStart)}
              onPaste={handlePaste}
              disabled={!conversation || submitting}
              placeholder={conversation ? `Message ${conversation.label}` : "Select a conversation"}
              aria-label="Message"
              rows={1}
              className="min-h-6 flex-1 resize-none border-0 bg-transparent px-0 py-0 text-sm leading-6 text-foreground placeholder:text-muted-foreground/80 focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
              style={{ height: "24px", overflow: "hidden" }}
            />
            <Button
              size="icon"
              className="h-8 w-8 shrink-0 self-end"
              type="submit"
              title="Send"
              aria-label="Send"
              disabled={!conversation || submitting || (trimmed === "" && attachments.length === 0)}
            >
              <Send className="h-4 w-4" />
            </Button>
          </div>
        </div>
        {error ? <p className="mt-1 text-sm text-destructive">{error}</p> : null}
      </form>
    </div>
  );
}

interface SlashCommandDefinition {
  kind: "command" | "skill";
  name: string;
  description: string;
  agentID?: string;
  agentHandle?: string;
  agentName?: string;
}

const slashCommands: SlashCommandDefinition[] = [
  { kind: "command", name: "new", description: "Start fresh context" },
  { kind: "command", name: "skills", description: "List available skills" },
  { kind: "command", name: "compact", description: "Compact Claude context" },
  { kind: "command", name: "plan", description: "Ask for an implementation plan" },
  { kind: "command", name: "init", description: "Initialize agent instructions" },
  { kind: "command", name: "model", description: "Set the agent model" },
  { kind: "command", name: "effort", description: "Set reasoning effort" },
  { kind: "command", name: "commit", description: "Commit workspace changes" },
  { kind: "command", name: "push", description: "Push the current branch" },
  { kind: "command", name: "review", description: "Review workspace changes" }
];

function buildSlashCommandOptions(
  groups: ConversationAgentSkills[],
  includeAgentLabel: boolean
): SlashCommandDefinition[] {
  const dynamic = groups.flatMap((group) =>
    group.skills
      .filter((skill) => !skill.conflicts_with_builtin)
      .map((skill) => skillCommandOption(group, skill, includeAgentLabel))
  );
  return [...slashCommands, ...dynamic];
}

function skillCommandOption(
  group: ConversationAgentSkills,
  skill: ConversationSkill,
  includeAgentLabel: boolean
): SlashCommandDefinition {
  const label = includeAgentLabel ? ` for @${group.agent_handle}` : "";
  return {
    kind: "skill",
    name: skill.name,
    description: skill.description || `Skill${label}`,
    agentID: group.agent_id,
    agentHandle: group.agent_handle,
    agentName: group.agent_name
  };
}

function slashCommandKey(command: SlashCommandDefinition): string {
  return `${command.kind}:${command.agentID ?? ""}:${command.name}`;
}

function commandLookupKey(value: string): string {
  return value.toLowerCase().replaceAll("_", "-");
}

function slashCommandIndicator(value: string, commands: SlashCommandDefinition[]) {
  if (!value.startsWith("/")) return null;
  const token = value.split(/\s+/, 1)[0] ?? "";
  const name = token.slice(1).toLowerCase();
  return slashCommandIndicatorForName(name, commands);
}

function slashCommandIndicatorForName(name: string, commands: SlashCommandDefinition[]) {
  if (!name) {
    return {
      status: "pending" as const,
      label: "Command",
      title: "Slash command"
    };
  }
  const lookupName = commandLookupKey(name);
  if (commands.some((command) => commandLookupKey(command.name) === lookupName)) {
    return {
      status: "recognized" as const,
      label: `/${name}`,
      title: `Recognized slash command /${name}`
    };
  }
  if (commands.some((command) => commandLookupKey(command.name).startsWith(lookupName))) {
    return {
      status: "pending" as const,
      label: `/${name}`,
      title: "Partial slash command"
    };
  }
  return {
    status: "unknown" as const,
    label: "Unknown",
    title: `Unknown slash command /${name}`
  };
}

function slashCommandTokenAt(value: string, caret: number) {
  if (caret < 0) return null;
  const beforeCaret = value.slice(0, caret);
  const match = /(^|\s)\/([A-Za-z0-9_-]*)$/.exec(beforeCaret);
  if (!match) return null;
  const prefix = match[1] ?? "";
  const start = match.index + prefix.length;
  let end = caret;
  while (end < value.length && !/\s/.test(value[end])) {
    end++;
  }
  return {
    start,
    end,
    query: value.slice(start + 1, end)
  };
}

function mentionTokenAt(value: string, caret: number) {
  if (caret < 0) return null;
  const beforeCaret = value.slice(0, caret);
  const match = /(^|[\s([{])@([A-Za-z0-9_-]*)$/.exec(beforeCaret);
  if (!match) return null;
  const prefix = match[1] ?? "";
  const start = match.index + prefix.length;
  return {
    start,
    end: caret,
    query: match[2] ?? ""
  };
}

function messageSenderLabel(
  message: Message,
  agents: Pick<Agent, "name" | "bot_user_id">[]
): string {
  if (message.sender_type === "user") {
    return "You";
  }
  if (message.sender_type === "system") {
    return "System";
  }
  const agent = agents.find((item) => item.bot_user_id === message.sender_id);
  return agent?.name ?? "Agent";
}

function messagePreview(body: string): string {
  const preview = body.replace(/\s+/g, " ").trim();
  return preview || "(empty)";
}

const maxDraftAttachments = 5;
const maxDraftAttachmentBytes = 10 * 1024 * 1024;
const maxDraftAttachmentTotalBytes = 25 * 1024 * 1024;
const draftAttachmentAccept = [
  "image/png",
  "image/jpeg",
  "image/webp",
  "image/gif",
  "text/*",
  ".txt",
  ".md",
  ".markdown",
  ".json",
  ".jsonl",
  ".csv",
  ".log",
  ".yaml",
  ".yml",
  ".toml",
  ".xml",
  ".html",
  ".css",
  ".js",
  ".jsx",
  ".ts",
  ".tsx",
  ".go",
  ".py",
  ".rs",
  ".java",
  ".c",
  ".cc",
  ".cpp",
  ".h",
  ".hpp",
  ".sh",
  ".sql",
  ".env",
  ".ini",
  ".conf"
].join(",");

function createDraftAttachment(file: File): DraftAttachment {
  const previewURL = isImageFile(file) ? URL.createObjectURL(file) : undefined;
  return {
    id: draftAttachmentID(),
    file,
    previewURL
  };
}

function draftAttachmentID(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function selectDraftAttachmentFiles(
  existingFiles: File[],
  incomingFiles: File[]
): { accepted: File[]; rejected: string[] } {
  const accepted: File[] = [];
  const rejected: string[] = [];
  let totalBytes = existingFiles.reduce((sum, file) => sum + file.size, 0);
  let remainingSlots = Math.max(0, maxDraftAttachments - existingFiles.length);

  for (const file of incomingFiles) {
    const name = file.name || "Attachment";
    if (remainingSlots <= 0) {
      rejected.push(`${name} exceeds the ${maxDraftAttachments} file limit`);
      continue;
    }
    if (file.size > maxDraftAttachmentBytes) {
      rejected.push(`${name} exceeds 10 MiB`);
      continue;
    }
    if (!isAcceptedDraftAttachment(file)) {
      rejected.push(`${name} is not a supported attachment type`);
      continue;
    }
    if (totalBytes + file.size > maxDraftAttachmentTotalBytes) {
      rejected.push(`${name} would exceed 25 MiB total`);
      continue;
    }
    accepted.push(file);
    remainingSlots -= 1;
    totalBytes += file.size;
  }

  return { accepted, rejected };
}

export function isAcceptedDraftAttachment(file: File): boolean {
  const type = file.type.toLowerCase();
  const ext = fileExtension(file.name);
  if (type === "image/svg+xml" || ext === ".svg") {
    return false;
  }
  if (isImageFile(file)) {
    return true;
  }
  if (type.startsWith("text/") || textDraftContentTypes.has(type)) {
    return true;
  }
  if (type === "" || type === "application/octet-stream") {
    return textDraftExtensions.has(ext);
  }
  return false;
}

function isImageFile(file: File): boolean {
  return imageDraftContentTypes.has(file.type.toLowerCase());
}

const imageDraftContentTypes = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
const textDraftContentTypes = new Set([
  "application/json",
  "application/x-ndjson",
  "application/xml",
  "application/yaml",
  "application/x-yaml",
  "application/toml"
]);
const textDraftExtensions = new Set([
  ".txt",
  ".md",
  ".markdown",
  ".json",
  ".jsonl",
  ".csv",
  ".log",
  ".yaml",
  ".yml",
  ".toml",
  ".xml",
  ".html",
  ".css",
  ".js",
  ".jsx",
  ".ts",
  ".tsx",
  ".go",
  ".py",
  ".rs",
  ".java",
  ".c",
  ".cc",
  ".cpp",
  ".h",
  ".hpp",
  ".sh",
  ".sql",
  ".env",
  ".ini",
  ".conf"
]);

function fileExtension(name: string): string {
  const lastDot = name.lastIndexOf(".");
  return lastDot >= 0 ? name.slice(lastDot).toLowerCase() : "";
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${Math.ceil(bytes / 1024)} KB`;
  }
  return `${bytes} B`;
}

function revokeAttachmentPreviews(attachments: DraftAttachment[]) {
  for (const attachment of attachments) {
    if (attachment.previewURL) {
      URL.revokeObjectURL(attachment.previewURL);
    }
  }
}

function hasDraggedFiles(event: DragEvent<HTMLDivElement>): boolean {
  return Array.from(event.dataTransfer.types).includes("Files");
}
