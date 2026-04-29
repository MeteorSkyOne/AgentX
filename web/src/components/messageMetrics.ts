import type { MessageMetricsSummary, UserPreferences } from "../api/types";

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
