import { useEffect, useState } from "react";
import { Check, Copy, Download } from "lucide-react";
import { exportThreadMarkdown } from "../../api/client";
import type { Thread } from "../../api/types";
import { copyTextToClipboard } from "../message-pane/utils";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";

export function postExportFilename(thread: Pick<Thread, "id" | "title">): string {
  const slug = thread.title
    .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, " ")
    .trim()
    .replace(/\s+/g, "-")
    .slice(0, 60);
  return slug ? `${slug}-${thread.id}.md` : `${thread.id}.md`;
}

export function CopyPostIDButton({ thread, className }: { thread: Pick<Thread, "id">; className?: string }) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 1500);
    return () => window.clearTimeout(timer);
  }, [copied]);

  async function copy() {
    try {
      await copyTextToClipboard(thread.id);
      setCopied(true);
    } catch {
      window.prompt("Copy post ID", thread.id);
    }
  }

  const label = copied ? "Post ID copied" : `Copy post ID (${thread.id})`;
  return (
    <Button
      variant="ghost"
      size="icon"
      className={className}
      title={label}
      aria-label={copied ? "Post ID copied" : "Copy post ID"}
      onClick={() => void copy()}
    >
      {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
    </Button>
  );
}

/** Header actions that let a post be referenced or exported outside AgentX. */
export function PostShareActions({ thread, buttonClassName }: { thread: Thread; buttonClassName?: string }) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [includeProcess, setIncludeProcess] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function exportMarkdown() {
    setExporting(true);
    setError(null);
    try {
      const blob = await exportThreadMarkdown(thread.id, { includeProcess });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = postExportFilename(thread);
      link.style.display = "none";
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      setDialogOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Export post failed");
    } finally {
      setExporting(false);
    }
  }

  return (
    <>
      <CopyPostIDButton thread={thread} className={buttonClassName} />
      <Button
        variant="ghost"
        size="icon"
        className={buttonClassName}
        title="Export post as Markdown"
        aria-label="Export post as Markdown"
        onClick={() => {
          setError(null);
          setDialogOpen(true);
        }}
      >
        <Download className="h-4 w-4" />
      </Button>
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Export post</DialogTitle>
            <DialogDescription>Download "{thread.title}" as Markdown.</DialogDescription>
          </DialogHeader>
          <label className="flex items-center gap-2 rounded-md border border-border bg-secondary/40 px-3 py-2 text-sm transition-colors hover:bg-accent/60">
            <Checkbox
              checked={includeProcess}
              onChange={(e) => setIncludeProcess(e.target.checked)}
              aria-label="Include thinking and tool calls"
            />
            Include thinking and tool calls
          </label>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={exporting}>
              Cancel
            </Button>
            <Button onClick={() => void exportMarkdown()} disabled={exporting}>
              <Download className="h-4 w-4" />
              {exporting ? "Exporting" : "Export"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
