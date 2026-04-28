import { RefreshCw } from "lucide-react";
import type { AgentProviderLimitWindow, AgentProviderLimits } from "../../api/types";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export function AgentProviderLimitsView({
  limits,
  error,
  isLoading,
  isFetching,
  onRefresh,
}: {
  limits?: AgentProviderLimits;
  error?: unknown;
  isLoading: boolean;
  isFetching: boolean;
  onRefresh: () => Promise<unknown> | void;
}) {
  const authDetails = limits
    ? [limits.auth.method, limits.auth.provider, limits.auth.plan].filter(Boolean).join(" · ")
    : "";
  const hasWindows = Boolean(limits && limits.windows.length > 0);
  const status = limits?.status ?? (error ? "error" : "unavailable");
  const message = providerLimitMessage(limits, error, isLoading);

  return (
    <section className="rounded-md border border-border bg-secondary/30 p-3" aria-label="Usage limits">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-xs font-medium uppercase text-muted-foreground">Usage limits</span>
            <Badge variant="outline" className={cn("shrink-0 text-[10px]", providerLimitStatusTone(status))}>
              {providerLimitStatusLabel(status)}
            </Badge>
          </div>
          {limits && (
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {limits.auth.logged_in ? "Signed in" : "Not signed in"}
              {authDetails ? ` · ${authDetails}` : ""}
            </p>
          )}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          title="Refresh usage limits"
          aria-label="Refresh usage limits"
          disabled={isFetching}
          onClick={() => {
            void onRefresh();
          }}
        >
          <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
        </Button>
      </div>

      {message && (
        <p className={cn("mt-2 text-xs", status === "error" ? "text-destructive" : "text-muted-foreground")}>
          {message}
        </p>
      )}

      {hasWindows && limits && (
        <div className="mt-3 space-y-3">
          {limits.windows.map((window) => (
            <ProviderLimitWindowBar key={`${window.kind}:${window.window_minutes}`} window={window} />
          ))}
        </div>
      )}
    </section>
  );
}

function ProviderLimitWindowBar({ window }: { window: AgentProviderLimitWindow }) {
  const percent = clampPercent(window.used_percent);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2 text-xs">
        <span className="truncate font-medium">{window.label}</span>
        <span className="shrink-0 text-muted-foreground">{formatProviderLimitPercent(window.used_percent)}</span>
      </div>
      <div
        className="h-2 overflow-hidden rounded-sm bg-muted"
        role="progressbar"
        aria-label={`${window.label} usage`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(percent)}
      >
        <div className={cn("h-full rounded-sm transition-[width]", usageBarColor(percent))} style={{ width: `${percent}%` }} />
      </div>
      <p className="truncate text-xs text-muted-foreground">
        {formatProviderLimitReset(window.resets_at)}
      </p>
    </div>
  );
}

export function formatProviderLimitPercent(value?: number | null): string {
  if (typeof value !== "number" || Number.isNaN(value)) return "unknown";
  return `${Math.round(clampPercent(value))}%`;
}

export function formatProviderLimitReset(resetsAt?: string | null, now = new Date()): string {
  if (!resetsAt) return "reset time unavailable";
  const date = new Date(resetsAt);
  if (Number.isNaN(date.getTime())) return "reset time unavailable";
  const diffMs = date.getTime() - now.getTime();
  if (diffMs <= 0) return "reset time passed";
  const totalMinutes = Math.max(1, Math.round(diffMs / 60_000));
  if (totalMinutes < 60) return `resets in ${totalMinutes}m`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours < 24) {
    return minutes > 0 ? `resets in ${hours}h ${minutes}m` : `resets in ${hours}h`;
  }
  const days = Math.floor(hours / 24);
  const remainingHours = hours % 24;
  return remainingHours > 0 ? `resets in ${days}d ${remainingHours}h` : `resets in ${days}d`;
}

function providerLimitMessage(
  limits: AgentProviderLimits | undefined,
  error: unknown,
  isLoading: boolean
): string {
  if (isLoading) return "Loading usage limits...";
  if (error instanceof Error) return error.message;
  if (error) return "Usage limits failed to load.";
  if (!limits) return "";
  if (limits.message) return limits.message;
  if (limits.status === "ok" && limits.windows.length > 0) return "";
  if (limits.status === "error") return "Usage limits failed to load.";
  if (limits.status === "unavailable") return "Usage limits are unavailable.";
  return "";
}

function providerLimitStatusLabel(status: AgentProviderLimits["status"]): string {
  switch (status) {
    case "ok":
      return "Live";
    case "error":
      return "Error";
    default:
      return "Unavailable";
  }
}

function providerLimitStatusTone(status: AgentProviderLimits["status"]): string {
  switch (status) {
    case "ok":
      return "border-emerald-500/40 text-emerald-600 dark:text-emerald-400";
    case "error":
      return "border-destructive/40 text-destructive";
    default:
      return "text-muted-foreground";
  }
}

function usageBarColor(percent: number): string {
  if (percent >= 95) return "bg-destructive";
  if (percent >= 80) return "bg-amber-500";
  return "bg-primary";
}

function clampPercent(value?: number | null): number {
  if (typeof value !== "number" || Number.isNaN(value)) return 0;
  return Math.max(0, Math.min(100, value));
}
