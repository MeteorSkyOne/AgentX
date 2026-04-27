import { useState } from "react";
import { MessageSquare, Pencil, Save, Trash2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import type { CreateThreadResponse, Thread } from "../../api/types";
import { formatDate } from "./utils";

export function ThreadForum({
  threads,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread
}: {
  threads: Thread[];
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

  async function submit() {
    if (!title.trim() || !body.trim()) return;
    setError(null);
    try {
      const created = await onCreateThread(title, body);
      setTitle("");
      setBody("");
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
        <Textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder="Body" aria-label="Post body" rows={3} />
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button className="w-full" onClick={submit}>Create post</Button>
      </div>
    </section>
  );
}
