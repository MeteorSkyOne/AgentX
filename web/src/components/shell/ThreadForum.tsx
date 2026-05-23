import { useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { skipToken, useQuery } from "@tanstack/react-query";
import { BookOpen, MessageSquare, Pencil, Save, Terminal, Trash2, X } from "lucide-react";
import { conversationSkills } from "../../api/client";
import type { Agent, ConversationType, CreateThreadResponse, Thread } from "../../api/types";
import {
  buildSlashCommandOptions,
  commandLookupKey,
  mentionDisplayName,
  mentionDisplayNamesToHandles,
  mentionTokenAt,
  slashCommandKey,
  slashCommandTokenAt,
  type SlashCommandDefinition
} from "../composerAutocomplete";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { formatDate } from "./utils";

export function ThreadForum({
  threads,
  conversation,
  mentionAgents = [],
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread
}: {
  threads: Thread[];
  conversation?: { type: ConversationType; id: string };
  mentionAgents?: Pick<Agent, "id" | "name" | "handle" | "kind" | "bot_user_id">[];
  onSelectThread: (thread: Thread) => void;
  onCreateThread: (title: string, body: string) => Promise<CreateThreadResponse>;
  onUpdateThread: (threadID: string, title: string) => Promise<Thread>;
  onDeleteThread: (thread: Thread) => Promise<void>;
}) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [draftTitle, setDraftTitle] = useState("");
  const [pendingID, setPendingID] = useState<string | null>(null);

  const [caret, setCaret] = useState(0);
  const [commandIndex, setCommandIndex] = useState(0);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [dismissedCommandKey, setDismissedCommandKey] = useState<string | null>(null);
  const [dismissedMentionKey, setDismissedMentionKey] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

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

  useEffect(() => {
    setMentionIndex(0);
  }, [mentionToken?.query]);

  useEffect(() => {
    setCommandIndex(0);
  }, [commandToken?.query]);

  useEffect(() => {
    if (!commandToken) setDismissedCommandKey(null);
  }, [commandToken]);

  useEffect(() => {
    if (!mentionToken) setDismissedMentionKey(null);
  }, [mentionToken]);

  function insertMention(agent: Pick<Agent, "name" | "handle">) {
    if (!mentionToken) return;
    const label = mentionDisplayName(agent);
    const next = `${body.slice(0, mentionToken.start)}@${label} ${body.slice(mentionToken.end)}`;
    const nextCaret = mentionToken.start + label.length + 2;
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
        ? `/${command.name} @${command.agentName ?? command.agentHandle}`
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
  }

  async function submit() {
    if (!title.trim() || !body.trim()) return;
    setError(null);
    const submittedBody = mentionDisplayNamesToHandles(body.trim(), mentionAgents);
    try {
      const created = await onCreateThread(title, submittedBody);
      setTitle("");
      setBody("");
      setCaret(0);
      onSelectThread(created.thread);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Post failed");
    }
  }

  function beginEdit(thread: Thread) {
    setEditingID(thread.id);
    setDraftTitle(thread.title);
    setError(null);
  }

  async function saveThread(thread: Thread) {
    const nextTitle = draftTitle.trim();
    if (!nextTitle) return;
    setPendingID(thread.id);
    setError(null);
    try {
      await onUpdateThread(thread.id, nextTitle);
      setEditingID(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update post failed");
    } finally {
      setPendingID(null);
    }
  }

  async function removeThread(thread: Thread) {
    if (!window.confirm(`Delete "${thread.title}"?`)) return;
    setPendingID(thread.id);
    setError(null);
    try {
      await onDeleteThread(thread);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete post failed");
    } finally {
      setPendingID(null);
    }
  }

  return (
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden p-3 md:p-4" aria-label="Threads">
      <ScrollArea className="min-h-0 flex-1">
        <div className="space-y-1">
          {threads.map((thread) => (
            <div
              key={thread.id}
              className="group flex min-h-10 w-full items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-accent/50 md:min-h-0"
            >
              {editingID === thread.id ? (
                <>
                  <Input
                    value={draftTitle}
                    onChange={(e) => setDraftTitle(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void saveThread(thread);
                      if (e.key === "Escape") setEditingID(null);
                    }}
                    aria-label="Post title"
                    className="h-8 flex-1"
                    autoFocus
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    title="Save post"
                    aria-label="Save post"
                    disabled={pendingID === thread.id || !draftTitle.trim()}
                    onClick={() => saveThread(thread)}
                  >
                    <Save className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    title="Cancel"
                    aria-label="Cancel"
                    disabled={pendingID === thread.id}
                    onClick={() => setEditingID(null)}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </>
              ) : (
                <>
                  <button
                    className="flex min-w-0 flex-1 items-center gap-3 rounded px-1 py-1 text-left"
                    onClick={() => onSelectThread(thread)}
                  >
                    <MessageSquare className="h-4 w-4 text-primary shrink-0" />
                    <span className="flex-1 truncate font-medium">{thread.title}</span>
                    <time className="hidden shrink-0 text-xs text-muted-foreground sm:block">{formatDate(thread.updated_at)}</time>
                  </button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-9 w-9 opacity-100 transition-opacity md:h-8 md:w-8 md:opacity-0 md:group-hover:opacity-100 focus:opacity-100"
                    title="Edit post"
                    aria-label="Edit post"
                    disabled={pendingID === thread.id}
                    onClick={() => beginEdit(thread)}
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-9 w-9 text-muted-foreground opacity-100 transition-opacity hover:text-destructive md:h-8 md:w-8 md:opacity-0 md:group-hover:opacity-100 focus:opacity-100"
                    title="Delete post"
                    aria-label="Delete post"
                    disabled={pendingID === thread.id}
                    onClick={() => removeThread(thread)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </>
              )}
            </div>
          ))}
          {threads.length === 0 && (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <MessageSquare className="h-10 w-10 mb-2" />
              <span className="text-sm">No posts yet</span>
            </div>
          )}
        </div>
      </ScrollArea>

      <div className="mt-4 shrink-0 space-y-2 rounded-lg border border-border p-3">
        <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Title" aria-label="Post title" />
        <div className="relative">
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
            placeholder="Body"
            aria-label="Post body"
            rows={3}
            className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-base md:text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
          />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button className="w-full" onClick={submit}>Create post</Button>
      </div>
    </section>
  );
}
