import { FileText, Image as ImageIcon, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { formatBytes, isImageFile, type DraftAttachment } from "./draft";

/**
 * AttachmentPreviews renders the chip list of staged draft attachments with
 * image thumbnails, file names, sizes, and a remove button. Shared by the
 * message composer and the thread/post creation form.
 */
export function AttachmentPreviews({
  attachments,
  onRemove,
  disabled = false,
  className
}: {
  attachments: DraftAttachment[];
  onRemove: (attachmentID: string) => void;
  disabled?: boolean;
  className?: string;
}) {
  if (attachments.length === 0) return null;
  return (
    <div className={cn("mb-2 flex max-h-28 flex-wrap gap-2 overflow-y-auto", className)}>
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
          <span className="shrink-0 text-muted-foreground">{formatBytes(attachment.file.size)}</span>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0"
            title="Remove attachment"
            aria-label="Remove attachment"
            disabled={disabled}
            onClick={() => onRemove(attachment.id)}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      ))}
    </div>
  );
}
