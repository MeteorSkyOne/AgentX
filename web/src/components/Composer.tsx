import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Send } from "lucide-react";
import { sendMessage } from "../api/client";
import type { Agent, ConversationType, Message } from "../api/types";
import { Button } from "@/components/ui/button";

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
  const [mentionIndex, setMentionIndex] = useState(0);
  const [dismissedMentionKey, setDismissedMentionKey] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const trimmed = body.trim();
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

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
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
          <div className="flex min-h-11 items-center gap-2 rounded-md border border-input bg-secondary/60 px-3 py-2 shadow-xs transition-[background-color,border-color,box-shadow] focus-within:border-ring focus-within:bg-background focus-within:ring-[3px] focus-within:ring-ring/20">
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
