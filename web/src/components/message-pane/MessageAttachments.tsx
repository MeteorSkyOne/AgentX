import { lazy, Suspense, useEffect, useRef, useState } from "react";
import type { PointerEvent, WheelEvent } from "react";
import { Download, Eye, FileText, Image as ImageIcon } from "lucide-react";
import { fetchAttachmentBlob } from "@/api/client";
import type { MessageAttachment } from "@/api/types";
import type { ThemeMode } from "@/theme";
import type { WorkspaceFileBrowserController } from "../WorkspaceFileBrowser";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

const LazyWorkspaceFileEditor = lazy(() =>
  import("../WorkspaceFileEditor").then((module) => ({ default: module.WorkspaceFileEditor }))
);

export function MessageAttachments({
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
  const canPreview = isTextAttachmentPreviewSupported(attachment);

  async function preview() {
    if (!canPreview) {
      return;
    }
    setBusy("preview");
    setError(null);
    try {
      const blob = await fetchAttachmentBlob(attachment.id);
      setPreviewBody(await blob.text());
      setPreviewOpen(true);
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
        {canPreview && (
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
        )}
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
              {attachment.content_type || "file attachment"} · {formatAttachmentBytes(attachment.size_bytes)}
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
    searchQuery: "",
    searchMode: "files",
    searchCaseSensitive: false,
    searchRegex: false,
    searchWholeWord: false,
    searchLoading: false,
    searchError: null,
    searchResults: [],
    searchTruncated: false,
    searchEngine: undefined,
    fileLoading: false,
    fileLoadError: null,
    fileSaving: false,
    fileDownloading: false,
    fileDeleting: false,
    entryActionPending: false,
    workspaceStatus: null,
    workspacePaneView: "files",
    gitEnabled: false,
    gitScope: "working_tree",
    gitTarget: "",
    gitCompare: "",
    gitHistoryMode: "repository",
    gitHistoryQuery: "",
    gitHistory: undefined,
    gitHistoryLoading: false,
    gitHistoryError: null,
    gitSelectedCommit: "",
    gitStatus: undefined,
    gitStatusLoading: false,
    gitStatusError: null,
    gitDiff: undefined,
    gitDiffLoading: false,
    gitDiffError: null,
    gitSelectedPath: "",
    fileOpenPosition: undefined,
    fileOpenRequestID: 0,
    fileViewMode: "edit",
    trimmedPath: attachment.filename.trim(),
    canUseWorkspace: false,
    canFetchFileBlob: false,
    canSearchWorkspace: false,
    tabs: [],
    activeTabId: null,
    activeTabEditorViewState: null,
    activeTabMarkdownPreviewScrollTop: 0,
    switchTab: noop,
    closeTab: noop,
    closeOtherTabs: noop,
    closeAllTabs: noop,
    pinTab: noop,
    reorderTabs: noop,
    setActiveTabEditorViewState: noop,
    saveTabEditorViewState: noop,
    saveTabMarkdownPreviewScrollTop: noop,
    setFilePath: noop,
    setFileBody: noop,
    setSearchQuery: noop,
    setSearchMode: noop,
    setSearchCaseSensitive: noop,
    setSearchRegex: noop,
    setSearchWholeWord: noop,
    setFileViewMode: noop,
    setWorkspacePaneView: noop,
    setGitScope: noop,
    setGitTarget: noop,
    setGitCompare: noop,
    setGitHistoryMode: noop,
    setGitHistoryQuery: noop,
    loadTree: asyncNoop,
    loadDirectory: asyncNoop,
    loadSearch: asyncNoop,
    clearSearch: noop,
    loadFile: asyncNoop,
    loadGitStatus: asyncNoop,
    loadGitHistory: asyncNoop,
    selectGitCommit: asyncNoop,
    loadGitDiff: asyncNoop,
    saveFile: asyncNoop,
    fetchFileBlob: async () => {
      throw new Error("File content is not available");
    },
    downloadFile: asyncNoop,
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

function formatAttachmentBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${Math.ceil(bytes / 1024)} KB`;
  }
  return `${bytes} B`;
}
