import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Send, Terminal } from "lucide-react";
import { sendMessage } from "../api/client";
import type { Agent, ConversationType, Message } from "../api/types";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface TypingAgent {
  name: string;
}

interface ComposerProps {
  conversation?: {
    type: ConversationType;
    id: string;
    label: string;
  };
  typingAgents?: TypingAgent[];
  mentionAgents?: Pick<Agent, "id" | "name" | "handle" | "kind">[];
  onSent: (message: Message) => void;
}

export function Composer({ conversation, typingAgents, mentionAgents = [], onSent }: ComposerProps) {
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [caret, setCaret] = useState(0);
  const [commandIndex, setCommandIndex] = useState(0);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [dismissedCommandKey, setDismissedCommandKey] = useState<string | null>(null);
  const [dismissedMentionKey, setDismissedMentionKey] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const trimmed = body.trim();
  const commandToken = useMemo(() => slashCommandTokenAt(body, caret), [body, caret]);
  const commandIndicator = useMemo(
    () => (commandToken ? slashCommandIndicatorForName(commandToken.query) : slashCommandIndicator(trimmed)),
    [commandToken, trimmed]
  );
  const commandKey = commandToken
    ? `${commandToken.start}:${commandToken.end}:${commandToken.query}`
    : null;
  const commandMatches = useMemo(() => {
    if (!commandToken) return [];
    const query = commandToken.query.toLowerCase();
    return slashCommands
      .filter((command) => command.name.startsWith(query))
      .slice(0, 8);
  }, [commandToken]);
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

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!conversation || trimmed === "") {
      return;
    }

    setError(null);
    setSubmitting(true);
    try {
      const message = await sendMessage(conversation.type, conversation.id, trimmed);
      setBody("");
      onSent(message);
    } catch (err) {
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
    const next = args ? `/${command.name} ${args}` : `/${command.name} `;
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
    <div className="shrink-0 border-t border-border bg-background/95 px-3 pt-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] md:px-4">
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
          {commandOpen && (
            <div className="absolute bottom-full left-0 mb-2 w-[min(22rem,calc(100vw-1.5rem))] overflow-hidden rounded-md border border-border bg-popover shadow-lg">
              {commandMatches.map((command, index) => (
                <button
                  key={command.name}
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
                  <Terminal className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">/{command.name}</span>
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
          <div
            className={cn(
              "flex min-h-11 items-center gap-2 rounded-md border border-input bg-secondary/60 px-3 py-2 shadow-xs transition-[background-color,border-color,box-shadow] focus-within:border-ring focus-within:bg-background focus-within:ring-[3px] focus-within:ring-ring/20",
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
              disabled={!conversation || submitting || trimmed === ""}
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
  name: string;
  description: string;
}

const slashCommands: SlashCommandDefinition[] = [
  { name: "new", description: "Start fresh context" },
  { name: "compact", description: "Compact Claude context" },
  { name: "plan", description: "Ask for an implementation plan" },
  { name: "init", description: "Initialize agent instructions" },
  { name: "model", description: "Set the agent model" },
  { name: "effort", description: "Set reasoning effort" },
  { name: "commit", description: "Commit workspace changes" },
  { name: "push", description: "Push the current branch" },
  { name: "review", description: "Review workspace changes" }
];

function slashCommandIndicator(value: string) {
  if (!value.startsWith("/")) return null;
  const token = value.split(/\s+/, 1)[0] ?? "";
  const name = token.slice(1).toLowerCase();
  return slashCommandIndicatorForName(name);
}

function slashCommandIndicatorForName(name: string) {
  if (!name) {
    return {
      status: "pending" as const,
      label: "Command",
      title: "Slash command"
    };
  }
  if (slashCommands.some((command) => command.name === name)) {
    return {
      status: "recognized" as const,
      label: `/${name}`,
      title: `Recognized slash command /${name}`
    };
  }
  if (slashCommands.some((command) => command.name.startsWith(name))) {
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
