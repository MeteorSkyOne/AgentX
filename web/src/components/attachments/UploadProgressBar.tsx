import type { UploadProgress } from "@/api/client";
import { cn } from "@/lib/utils";
import { formatBytes } from "./draft";

/**
 * UploadProgressBar reports attachment upload progress for the message composer
 * and the thread/post creation form. Attachments have no size cap, so an upload
 * can run long enough that it needs to show it is still moving.
 *
 * Once the body is on the wire the server still has to store it, so a full bar
 * switches to a processing label instead of claiming the send is done.
 */
export function UploadProgressBar({
  progress,
  className
}: {
  progress: UploadProgress | null;
  className?: string;
}) {
  if (!progress) return null;

  const determinate = progress.total > 0;
  const processing = determinate && progress.percent >= 100;
  const label = processing
    ? "Processing upload…"
    : determinate
      ? `Uploading ${formatBytes(progress.loaded)} of ${formatBytes(progress.total)}`
      : "Uploading…";

  return (
    <div className={cn("mb-2 space-y-1", className)}>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="truncate" aria-live="polite">
          {label}
        </span>
        {determinate && !processing ? (
          <span className="shrink-0 tabular-nums">{progress.percent}%</span>
        ) : null}
      </div>
      <div
        className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
        role="progressbar"
        aria-label="Upload progress"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={determinate ? progress.percent : undefined}
      >
        <div
          className={cn(
            "h-full rounded-full bg-primary transition-[width] duration-150",
            determinate ? "" : "w-full animate-pulse"
          )}
          style={determinate ? { width: `${progress.percent}%` } : undefined}
        />
      </div>
    </div>
  );
}
