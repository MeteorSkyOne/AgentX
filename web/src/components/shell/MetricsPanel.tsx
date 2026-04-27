import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { BarChart3 } from "lucide-react";
import { channelMetrics, conversationMetrics, projectMetrics } from "../../api/client";
import type {
  AgentRunMetric,
  Channel,
  ConversationType,
  MetricsProvider,
  Project,
} from "../../api/types";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { formatMilliseconds, formatTPS } from "../messageMetrics";
import type { ActiveConversation } from "./types";

type MetricsScope = "conversation" | "channel" | "project";
type ProviderFilter = MetricsProvider | "";

interface MetricsPanelProps {
  project?: Project;
  selectedChannel?: Channel;
  activeConversation?: ActiveConversation;
}

export function MetricsPanel({ project, selectedChannel, activeConversation }: MetricsPanelProps) {
  const [scope, setScope] = useState<MetricsScope>(() =>
    activeConversation ? "conversation" : selectedChannel ? "channel" : "project"
  );
  const [provider, setProvider] = useState<ProviderFilter>("");

  const scopeOptions = useMemo(
    () => [
      { id: "conversation" as const, label: "Conversation", enabled: Boolean(activeConversation) },
      { id: "channel" as const, label: "Channel", enabled: Boolean(selectedChannel) },
      { id: "project" as const, label: "Project", enabled: Boolean(project) },
    ],
    [activeConversation, selectedChannel, project]
  );

  useEffect(() => {
    const preferred: MetricsScope = activeConversation
      ? "conversation"
      : selectedChannel
        ? "channel"
        : "project";
    if (!scopeOptions.find((item) => item.id === scope)?.enabled) {
      setScope(preferred);
    }
  }, [activeConversation?.id, selectedChannel?.id, project?.id, scope, scopeOptions]);

  const metricsQuery = useQuery({
    queryKey: [
      "metrics",
      scope,
      activeConversation?.type,
      activeConversation?.id,
      selectedChannel?.id,
      project?.id,
      provider,
    ],
    queryFn: () => loadMetrics(scope, {
      activeConversation,
      selectedChannel,
      project,
      provider,
    }),
    enabled: canLoadScope(scope, activeConversation, selectedChannel, project),
  });

  const rows = metricsQuery.data ?? [];

  return (
    <section className="flex min-h-0 flex-1 flex-col bg-background" data-testid="metrics-panel">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border px-3 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="inline-flex rounded-md border border-border bg-muted/30 p-0.5">
            {scopeOptions.map((item) => (
              <Button
                key={item.id}
                type="button"
                variant={scope === item.id ? "secondary" : "ghost"}
                size="sm"
                className={cn("h-8 rounded-[5px] px-3", scope === item.id && "shadow-xs")}
                disabled={!item.enabled}
                onClick={() => setScope(item.id)}
              >
                {item.label}
              </Button>
            ))}
          </div>
          <Select
            value={provider}
            onChange={(event) => setProvider(event.target.value as ProviderFilter)}
            aria-label="Provider"
            className="w-40"
          >
            <option value="">All providers</option>
            <option value="claude">Claude Code</option>
            <option value="codex">Codex</option>
            <option value="fake">Fake</option>
          </Select>
        </div>
        <span className="text-xs text-muted-foreground">{metricsQuery.isFetching ? "Loading" : `${rows.length} agents`}</span>
      </div>

      {metricsQuery.isLoading ? (
        <div className="flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground">
          Loading metrics...
        </div>
      ) : metricsQuery.isError ? (
        <div className="flex min-h-0 flex-1 items-center justify-center text-sm text-destructive">
          {metricsQuery.error instanceof Error ? metricsQuery.error.message : "Failed to load metrics"}
        </div>
      ) : rows.length === 0 ? (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
          <BarChart3 className="h-10 w-10" />
          <span>No metrics yet</span>
        </div>
      ) : (
        <ScrollArea className="min-h-0 flex-1" orientation="both">
          <div className="min-w-[1280px] p-3">
            <table className="w-full border-collapse text-left text-xs">
              <thead className="text-muted-foreground">
                <tr className="border-b border-border">
                  {[
                    "Last Run",
                    "Scope",
                    "Agent",
                    "Provider",
                    "Model",
                    "Runs",
                    "Completed",
                    "Failed",
                    "Avg TTFT",
                    "TPS",
                    "Avg Duration",
                    "Input",
                    "Cached",
                    "Cache Hit",
                    "Output",
                    "Reasoning",
                    "Total",
                    "Cost",
                  ].map((header) => (
                    <th key={header} className="whitespace-nowrap px-2 py-2 font-medium">
                      {header}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={`${row.project_id}:${row.channel_id}:${row.thread_id}:${row.conversation_id}:${row.agent_id}:${row.provider}`} className="border-b border-border/60 hover:bg-accent/25">
                    <td className="whitespace-nowrap px-2 py-2">{formatDateTime(row.started_at)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{scopeLabel(row)}</td>
                    <td className="whitespace-nowrap px-2 py-2 font-medium">{row.agent_name || shortID(row.agent_id)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{providerLabel(row.provider)}</td>
                    <td className="max-w-40 truncate px-2 py-2 font-mono text-[11px] text-muted-foreground">
                      {row.model || "default"}
                    </td>
                    <td className="whitespace-nowrap px-2 py-2">{formatInteger(row.run_count)}</td>
                    <td className="whitespace-nowrap px-2 py-2 text-emerald-600">{formatInteger(row.completed_runs)}</td>
                    <td className="whitespace-nowrap px-2 py-2 text-destructive">{formatInteger(row.failed_runs)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatMetricMS(row.ttft_ms)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatMetricTPS(row.tps)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatMetricMS(row.duration_ms)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatInteger(row.input_tokens)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatInteger(row.cached_input_tokens ?? row.cache_read_input_tokens)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatPercent(row.cache_hit_rate)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatInteger(row.output_tokens)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatInteger(row.reasoning_output_tokens)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatInteger(row.total_tokens)}</td>
                    <td className="whitespace-nowrap px-2 py-2">{formatCost(row.total_cost_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </ScrollArea>
      )}
    </section>
  );
}

function loadMetrics(
  scope: MetricsScope,
  context: {
    activeConversation?: ActiveConversation;
    selectedChannel?: Channel;
    project?: Project;
    provider: ProviderFilter;
  }
): Promise<AgentRunMetric[]> {
  const options = { limit: 100, provider: context.provider, group: "agent" as const };
  if (scope === "conversation" && context.activeConversation) {
    return conversationMetrics(
      context.activeConversation.type as ConversationType,
      context.activeConversation.id,
      options
    );
  }
  if (scope === "channel" && context.selectedChannel) {
    return channelMetrics(context.selectedChannel.id, options);
  }
  if (scope === "project" && context.project) {
    return projectMetrics(context.project.id, options);
  }
  return Promise.resolve([]);
}

function canLoadScope(
  scope: MetricsScope,
  activeConversation?: ActiveConversation,
  selectedChannel?: Channel,
  project?: Project
): boolean {
  if (scope === "conversation") return Boolean(activeConversation);
  if (scope === "channel") return Boolean(selectedChannel);
  return Boolean(project);
}

function providerLabel(provider: string): string {
  switch (provider) {
    case "claude":
      return "Claude Code";
    case "codex":
      return "Codex";
    case "fake":
      return "Fake";
    default:
      return provider || "Unknown";
  }
}

function scopeLabel(row: AgentRunMetric): string {
  if (row.thread_id) {
    return row.thread_title ? `Thread ${row.thread_title}` : `Thread ${shortID(row.thread_id)}`;
  }
  if (row.channel_id) {
    return row.channel_name ? `#${row.channel_name}` : `Channel ${shortID(row.channel_id)}`;
  }
  if (row.project_id) {
    return row.project_name ? `Project ${row.project_name}` : `Project ${shortID(row.project_id)}`;
  }
  return `${row.conversation_type} ${shortID(row.conversation_id)}`;
}

function shortID(value: string | undefined): string {
  if (!value) return "";
  return value.length > 14 ? `${value.slice(0, 8)}...${value.slice(-4)}` : value;
}

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatMetricMS(value: number | null | undefined): string {
  return isFiniteNumber(value) ? formatMilliseconds(value) : "-";
}

function formatMetricTPS(value: number | null | undefined): string {
  return isFiniteNumber(value) ? formatTPS(value) : "-";
}

function formatInteger(value: number | null | undefined): string {
  return isFiniteNumber(value) ? new Intl.NumberFormat().format(value) : "-";
}

function formatPercent(value: number | null | undefined): string {
  if (!isFiniteNumber(value)) return "-";
  const clamped = Math.min(1, Math.max(0, value));
  return `${Math.round(clamped * 100)}%`;
}

function formatCost(value: number | null | undefined): string {
  if (!isFiniteNumber(value)) return "-";
  if (value === 0) return "$0";
  return `$${value.toFixed(value < 0.01 ? 4 : 2)}`;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
