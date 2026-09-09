import type { MessageContextUsage, MessageMetricsSummary, UserPreferences } from "../api/types";

export function messageMetricsParts(
  metrics: MessageMetricsSummary | undefined,
  preferences: Pick<UserPreferences, "show_ttft" | "show_tps">
): string[] {
  if (!metrics) {
    return [];
  }
  const parts: string[] = [];
  if (preferences.show_ttft && isFiniteNumber(metrics.ttft_ms)) {
    parts.push(`TTFT ${formatMilliseconds(metrics.ttft_ms)}`);
  }
  if (preferences.show_tps && isFiniteNumber(metrics.tps)) {
    parts.push(`TPS ${formatTPS(metrics.tps)}`);
  }
  return parts;
}

/**
 * Footer label for the context-window fill after a reply, e.g. "76k/200k (38%)".
 * Returns null when the agent reported nothing usable.
 */
export function contextUsageLabel(usage: MessageContextUsage | undefined): string | null {
  if (!usage) {
    return null;
  }
  const used = isFiniteNumber(usage.total_tokens) ? usage.total_tokens : null;
  const window = isFiniteNumber(usage.context_window_tokens) && usage.context_window_tokens > 0
    ? usage.context_window_tokens
    : null;
  let percent = isFiniteNumber(usage.used_percent) ? usage.used_percent : null;
  if (percent === null && used !== null && window !== null) {
    percent = (used / window) * 100;
  }
  if (used === null) {
    return percent === null ? null : `Context ${formatPercent(percent)}`;
  }
  const ratio = window === null ? formatTokenCount(used) : `${formatTokenCount(used)}/${formatTokenCount(window)}`;
  return percent === null ? `Context ${ratio}` : `Context ${ratio} (${formatPercent(percent)})`;
}

export function formatTokenCount(value: number): string {
  if (value >= 1_000_000) {
    return `${formatTrimmedNumber(Math.round((value / 1_000_000) * 10) / 10)}m`;
  }
  if (value >= 1000) {
    return `${Math.round(value / 1000)}k`;
  }
  return String(Math.round(value));
}

function formatPercent(value: number): string {
  return `${Math.round(Math.max(0, value))}%`;
}

export function messageWorkingLabel(metrics: MessageMetricsSummary | undefined): string | null {
  if (!metrics) {
    return null;
  }
  if (isFiniteNumber(metrics.duration_ms)) {
    return workingDurationLabel(metrics.duration_ms);
  }
  if (!metrics.started_at || !metrics.completed_at) {
    return null;
  }
  return workingDurationBetween(metrics.started_at, metrics.completed_at);
}

export function workingDurationBetween(
  startedAt: string | undefined,
  endedAt?: string | Date,
  now: Date = new Date()
): string | null {
  if (!startedAt) {
    return null;
  }
  const started = Date.parse(startedAt);
  const ended = endedAt
    ? endedAt instanceof Date
      ? endedAt.getTime()
      : Date.parse(endedAt)
    : now.getTime();
  if (!Number.isFinite(started) || !Number.isFinite(ended)) {
    return null;
  }
  return workingDurationLabel(Math.max(0, ended - started));
}

export function workingDurationLabel(durationMs: number): string | null {
  if (!Number.isFinite(durationMs) || durationMs < 0) {
    return null;
  }
  return `Working ${formatDuration(durationMs)}`;
}

export function formatDuration(durationMs: number): string {
  const totalSeconds = durationMs / 1000;
  if (totalSeconds < 60) {
    const rounded = totalSeconds < 10 ? Math.round(totalSeconds * 10) / 10 : Math.round(totalSeconds);
    return `${formatTrimmedNumber(rounded)}s`;
  }

  const wholeSeconds = Math.floor(totalSeconds);
  const hours = Math.floor(wholeSeconds / 3600);
  const minutes = Math.floor((wholeSeconds % 3600) / 60);
  const seconds = wholeSeconds % 60;
  const parts: string[] = [];
  if (hours > 0) {
    parts.push(`${hours}h`);
  }
  if (minutes > 0 || hours > 0) {
    parts.push(`${minutes}m`);
  }
  parts.push(`${seconds}s`);
  return parts.join(" ");
}

export function formatMilliseconds(value: number): string {
  if (value >= 1000) {
    return `${(value / 1000).toFixed(value >= 10_000 ? 0 : 1)}s`;
  }
  return `${Math.round(value)}ms`;
}

export function formatTPS(value: number): string {
  if (value >= 100) {
    return value.toFixed(0);
  }
  if (value >= 10) {
    return value.toFixed(1);
  }
  return value.toFixed(2);
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function formatTrimmedNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}
