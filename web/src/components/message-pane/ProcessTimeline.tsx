import { Fragment, useEffect, useMemo, useState } from "react";
import {
  Bot,
  Brain,
  Braces,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Square,
  Wrench,
} from "lucide-react";
import { fetchMessageProcessItem } from "@/api/client";
import type { Message, MessageProcessItemDetail, ProcessItem } from "@/api/types";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import { cn } from "@/lib/utils";
import { AgentAvatar, agentKindColor } from "../AgentAvatar";
import { workingDurationBetween } from "../messageMetrics";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { ScrollArea } from "@/components/ui/scroll-area";
import { MessageMarkdown, messageBodyClassName } from "./markdown";
import type { DisplayProcessEntry, DisplayProcessItem, DisplayProcessToolFragment, StreamingMessage } from "./types";

export function StreamingItem({
  item,
  agentName,
  agentKind,
  agentID,
  hideAvatar,
  workspacePath,
  onOpenWorkspacePath,
  onStopSubagent,
}: {
  item: StreamingMessage;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
  hideAvatar?: boolean;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
  onStopSubagent?: (toolCallID: string) => Promise<void>;
}) {
  const isError = Boolean(item.error);
  const label = isError ? "System" : agentName ?? "Agent";
  const process = processFromStreaming(item);
  const workingLabel = useStreamingWorkingLabel(item.startedAt, item.endedAt);
  const subagents = useMemo(() => getSubagentSummary(process), [process]);

  return (
    <div className={cn("group flex min-w-0 max-w-full gap-3 rounded-md px-1 py-1 md:gap-4 md:px-2", isError && "opacity-70")}>
      {!hideAvatar && (
        isError ? (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className="text-white text-sm bg-destructive">
              <CircleAlert className="h-5 w-5" />
            </AvatarFallback>
          </Avatar>
        ) : agentID ? (
          <AgentAvatar agentID={agentID} kind={agentKind ?? "fake"} size="md" className="shrink-0" />
        ) : (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className={cn("text-white text-sm", agentKindColor(agentKind ?? "fake"))}>
              <CircleAlert className="h-5 w-5" />
            </AvatarFallback>
          </Avatar>
        )
      )}

      <div className="min-w-0 flex-1 select-text space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{label}</span>
          {!isError && (
            <Badge variant="secondary" className="text-xs">
              BOT
            </Badge>
          )}
          {item.team && (
            <Badge variant="outline" className="text-xs">
              TEAM
            </Badge>
          )}
          <span className="text-xs text-muted-foreground animate-pulse">streaming...</span>
        </div>
        <InterleavedMessageBody
          text={item.error ?? item.text}
          processItems={process}
          workspacePath={workspacePath}
          onOpenWorkspacePath={onOpenWorkspacePath}
          className={messageBodyClassName}
          defaultProcessOpen
          onStopSubagent={onStopSubagent}
        />
        {(workingLabel || subagents.length > 0) && (
          <div className="space-y-1">
            <div className="flex items-center gap-2 text-[11px] font-medium text-muted-foreground">
              {workingLabel && <span>{workingLabel}</span>}
              <SubagentBadge subagents={subagents} onStopSubagent={onStopSubagent} />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/** Asks the backend to stop a running subagent; the completed signal that the
 * CLI emits in response is what flips the UI to done. */
function StopSubagentButton({
  toolCallID,
  onStopSubagent,
}: {
  toolCallID: string;
  onStopSubagent: (toolCallID: string) => Promise<void>;
}) {
  const [stopping, setStopping] = useState(false);
  return (
    <button
      type="button"
      title="Stop subagent"
      disabled={stopping}
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded bg-destructive/10 px-1.5 py-0.5 text-[11px] font-medium text-destructive transition-colors hover:bg-destructive/20",
        stopping && "opacity-50"
      )}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        setStopping(true);
        void onStopSubagent(toolCallID).catch(() => setStopping(false));
      }}
    >
      <Square className="h-2.5 w-2.5 fill-current" />
      <span>{stopping ? "stopping…" : "stop"}</span>
    </button>
  );
}

function SubagentBadge({
  subagents,
  onStopSubagent,
}: {
  subagents: SubagentInfo[];
  onStopSubagent?: (toolCallID: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const activeCount = subagents.filter((s) => s.active).length;
  const totalCount = subagents.length;

  if (totalCount === 0) return null;

  const label =
    activeCount > 0
      ? `${activeCount} subagent${activeCount !== 1 ? "s" : ""}`
      : `${totalCount} subagent${totalCount !== 1 ? "s" : ""} done`;

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className={cn(
            "inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium transition-colors",
            activeCount > 0
              ? "bg-blue-500/10 text-blue-400 hover:bg-blue-500/20"
              : "bg-muted text-muted-foreground hover:bg-muted/80"
          )}
        >
          <Bot className="h-3 w-3" />
          <span>{label}</span>
          {open ? <ChevronDown className="h-2.5 w-2.5" /> : <ChevronRight className="h-2.5 w-2.5" />}
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-1 space-y-1 rounded-md border border-border/60 bg-background/50 p-2 text-xs">
          {subagents.map((sa) => (
            <div
              key={sa.toolCallID}
              className={cn(
                "flex items-center gap-2 rounded px-2 py-1",
                sa.active ? "text-foreground" : "text-muted-foreground/60"
              )}
            >
              <span
                className={cn(
                  "flex h-4 w-4 shrink-0 items-center justify-center rounded-full",
                  sa.active ? "bg-blue-500/10 text-blue-400" : "bg-emerald-500/10 text-emerald-400"
                )}
              >
                {sa.active ? (
                  <Bot className="h-2.5 w-2.5 animate-pulse" />
                ) : (
                  <CheckCircle2 className="h-2.5 w-2.5" />
                )}
              </span>
              <span className="min-w-0 flex-1 truncate">{sa.description}</span>
              {sa.active && onStopSubagent && (
                <StopSubagentButton toolCallID={sa.toolCallID} onStopSubagent={onStopSubagent} />
              )}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ProcessBlock({
  items,
  subagentGroups = [],
  defaultOpen = true,
  messageID,
  onStopSubagent,
}: {
  items: ProcessItem[];
  subagentGroups?: SubagentGroup[];
  defaultOpen?: boolean;
  messageID?: string;
  onStopSubagent?: (toolCallID: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const displayItems = useMemo(() => groupProcessFragments(mergeToolProcessItems(items)), [items]);
  if (displayItems.length === 0 && subagentGroups.length === 0) {
    return null;
  }
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground py-1">
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        <Brain className="h-3 w-3" />
        <span>Process</span>
        <span className="text-[10px] text-muted-foreground/70">
          {displayItems.length + subagentGroups.length}
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="space-y-2 border-l border-border/60 pl-3 py-1">
          {displayItems.map((item, index) => (
            <ProcessEntryRow key={processEntryKey(item, index)} item={item} messageID={messageID} />
          ))}
          {subagentGroups.map((group) => (
            <SubagentSection
              key={group.parentToolCallID}
              group={group}
              messageID={messageID}
              onStopSubagent={onStopSubagent}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

/** A subagent's own work, shown under the call that spawned it. */
function SubagentSection({
  group,
  messageID,
  onStopSubagent,
}: {
  group: SubagentGroup;
  messageID?: string;
  onStopSubagent?: (toolCallID: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const displayItems = useMemo(() => mergeToolProcessItems(group.items), [group.items]);
  const toolCount = displayItems.filter((item) => item.type !== "thinking").length;
  const label = toolCount === 1 ? "1 tool" : `${toolCount} tools`;

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border border-border/60 bg-muted/15 p-2.5 text-xs"
    >
      <div className="flex w-full min-w-0 items-center gap-2">
        <CollapsibleTrigger className="flex min-w-0 flex-1 items-center gap-2 text-left text-muted-foreground hover:text-foreground">
          {open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
          <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-purple-500/10 text-purple-400">
            <Bot className={cn("h-3.5 w-3.5", group.active && "animate-pulse")} />
          </span>
          <span className="font-medium text-foreground">Subagent</span>
          <span className="min-w-0 truncate text-[11px]">{group.description}</span>
          {group.active ? (
            <span className="ml-auto flex shrink-0 items-center gap-1 rounded bg-blue-500/10 px-1.5 py-0.5 text-[11px] text-blue-400">
              running
            </span>
          ) : (
            <span className="ml-auto flex shrink-0 items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-[11px]">
              <CheckCircle2 className="h-2.5 w-2.5 text-emerald-400" />
              done
            </span>
          )}
          <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px]">{label}</span>
        </CollapsibleTrigger>
        {group.active && onStopSubagent && (
          <StopSubagentButton toolCallID={group.parentToolCallID} onStopSubagent={onStopSubagent} />
        )}
      </div>
      <CollapsibleContent className="space-y-2 pt-2">
        {displayItems.map((item, index) => (
          <ProcessRow key={processItemKey(item, index)} item={item} messageID={messageID} />
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

function ProcessEntryRow({ item, messageID }: { item: DisplayProcessEntry; messageID?: string }) {
  if (item.type === "tool_fragment") {
    return <ToolFragmentRow item={item} messageID={messageID} />;
  }
  return <ProcessRow item={item} messageID={messageID} />;
}

function ProcessRow({ item, messageID }: { item: DisplayProcessItem; messageID?: string }) {
  if (item.type === "thinking") {
    return (
      <div className="space-y-1 text-xs text-muted-foreground">
        <div className="flex items-center gap-1.5 font-medium not-italic">
          <Brain className="h-3 w-3" />
          <span>Thinking</span>
        </div>
        {item.text && (
          <div className="rounded-md bg-muted/30 px-3 py-2 italic">
            <MessageMarkdown text={item.text} />
          </div>
        )}
      </div>
    );
  }

  return <ToolProcessRow item={item} messageID={messageID} />;
}

function ToolFragmentRow({ item, messageID }: { item: DisplayProcessToolFragment; messageID?: string }) {
  const [open, setOpen] = useState(false);
  const label = item.items.length === 1 ? "1 tool" : `${item.items.length} tools`;
  const names = toolFragmentNames(item.items);

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border border-border/60 bg-muted/15 p-2.5 text-xs"
    >
      <CollapsibleTrigger className="flex w-full min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground">
        {open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
        <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-blue-500/10 text-blue-400">
          <Wrench className="h-3.5 w-3.5" />
        </span>
        <span className="font-medium text-foreground">Tools</span>
        <span className="rounded bg-muted px-1.5 py-0.5 text-[11px]">{label}</span>
        {names && <span className="min-w-0 truncate text-[11px]">{names}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 pt-2">
        {item.items.map((tool, index) => (
          <ToolProcessRow key={processItemKey(tool, index)} item={tool} messageID={messageID} />
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

function ToolProcessRow({ item, messageID }: { item: DisplayProcessItem; messageID?: string }) {
  const [detail, setDetail] = useState<DisplayProcessItem | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const canLazyLoad =
    Boolean(messageID) && item.has_detail === true && typeof item.process_index === "number";
  const activeItem = detail ?? item;
  const resultItem = activeItem.result ?? (activeItem.type === "tool_result" ? activeItem : undefined);
  const isResultOnly = activeItem.type === "tool_result" && !activeItem.result;
  const hasResult = Boolean(resultItem);
  const status = resultItem?.status ?? activeItem.status;
  const isError = status === "error";
  const [open, setOpen] = useState(!hasResult && !canLazyLoad);
  const raw = rawProcessValue(activeItem);
  const preview = toolPreview(activeItem);

  useEffect(() => {
    setDetail(null);
    setDetailError(null);
    setDetailLoading(false);
  }, [messageID, item.process_index]);

  useEffect(() => {
    if (!open || !canLazyLoad || detail || detailLoading || detailError) {
      return;
    }
    let cancelled = false;
    setDetailLoading(true);
    fetchMessageProcessItem(messageID!, item.process_index!)
      .then((loaded) => {
        if (!cancelled) {
          setDetail(displayProcessItemFromDetail(loaded));
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setDetailError(err instanceof Error ? err.message : "Tool details failed");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [canLazyLoad, detail, detailError, item.process_index, messageID, open]);

  function retryDetailLoad() {
    setDetailError(null);
    setDetail(null);
    setOpen(true);
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border border-border/60 bg-background/35 p-2.5 text-xs"
    >
      <CollapsibleTrigger className="flex w-full flex-wrap items-center gap-2 text-left">
        {open ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
        <span
          className={cn(
            "flex h-5 w-5 items-center justify-center rounded-full",
            isError
              ? "bg-destructive/10 text-destructive"
              : hasResult
                ? "bg-emerald-500/10 text-emerald-400"
                : "bg-blue-500/10 text-blue-400"
          )}
        >
          {isError ? (
            <CircleAlert className="h-3.5 w-3.5" />
          ) : hasResult ? (
            <CheckCircle2 className="h-3.5 w-3.5" />
          ) : (
            <Wrench className="h-3.5 w-3.5" />
          )}
        </span>
        <span className="font-medium text-foreground">
          {activeItem.result ? "Tool" : isResultOnly ? "Tool result" : "Tool call"}
        </span>
        {activeItem.tool_name && <span className="rounded bg-muted px-1.5 py-0.5 text-muted-foreground">{activeItem.tool_name}</span>}
        {status && (
          <span className={cn("rounded px-1.5 py-0.5", isError ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")}>
            {status}
          </span>
        )}
        {activeItem.tool_call_id && <span className="font-mono text-[10px] text-muted-foreground">{shortID(activeItem.tool_call_id)}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 pt-2">
        {detailLoading && (
          <div className="rounded-md bg-muted/30 px-3 py-2 text-[11px] text-muted-foreground">
            Loading tool details...
          </div>
        )}
        {detailError && (
          <div className="flex flex-wrap items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-[11px] text-destructive">
            <span>{detailError}</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[11px]"
              onClick={retryDetailLoad}
            >
              Retry
            </Button>
          </div>
        )}
        {preview.title && <div className="font-medium text-muted-foreground">{preview.title}</div>}
        {preview.command && (
          <ScrollArea orientation="horizontal" className="rounded-md bg-sidebar/80">
            <pre className="min-w-max px-3 py-2 font-mono text-[12px] leading-relaxed text-foreground">
              {preview.command}
            </pre>
          </ScrollArea>
        )}
        {preview.body && (
          <ScrollArea className="max-h-64 rounded-md bg-sidebar/80" viewportClassName="max-h-64">
            <pre className="whitespace-pre-wrap px-3 py-2 font-mono text-[12px] leading-relaxed text-muted-foreground">
              {preview.body}
            </pre>
          </ScrollArea>
        )}
        {preview.params.length > 0 && (
          <div className="grid gap-1.5">
            {preview.params.map(([key, value]) => (
              <div key={key} className="grid grid-cols-[6rem_1fr] gap-2 rounded bg-muted/25 px-2 py-1">
                <span className="truncate text-muted-foreground">{key}</span>
                <span className="min-w-0 truncate font-mono text-[11px] text-foreground">{value}</span>
              </div>
            ))}
          </div>
        )}
        {raw !== undefined && (
          <details className="group">
            <summary className="flex cursor-pointer list-none items-center gap-1.5 text-[11px] text-muted-foreground hover:text-foreground">
              <Braces className="h-3 w-3" />
              <span>Details</span>
            </summary>
            <ScrollArea orientation="both" className="mt-1 max-h-80 rounded-md bg-sidebar" viewportClassName="max-h-80">
              <pre className="min-w-max p-3 text-[11px] leading-relaxed text-muted-foreground">
                {formatJSON(raw)}
              </pre>
            </ScrollArea>
          </details>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

interface ToolPreview {
  title?: string;
  command?: string;
  body?: string;
  params: Array<[string, string]>;
}

function toolPreview(item: DisplayProcessItem): ToolPreview {
  const resultItem = item.result ?? (item.type === "tool_result" ? item : undefined);
  const inputValue = item.type === "tool_result" && !item.result ? item.input : item.input ?? resultItem?.input;
  const outputValue = resultItem?.output ?? resultItem?.text ?? resultItem?.raw;
  const inputRaw = item.raw;
  const resultRaw = resultItem?.raw;
  const command = extractCommand(inputValue) ?? extractCommand(inputRaw) ?? extractCommand(resultRaw);
  const cwd =
    extractString(inputValue, "cwd", "workdir", "working_directory") ??
    extractString(inputRaw, "cwd", "workdir", "working_directory") ??
    extractString(resultRaw, "cwd", "workdir", "working_directory");
  const description =
    extractString(inputValue, "description", "reason") ??
    extractString(inputRaw, "description", "reason") ??
    extractString(resultRaw, "description", "reason");
  const body = resultItem ? toolOutputText(outputValue) : undefined;
  const params = previewParams(inputValue, new Set(["command", "cmd", "cwd", "workdir", "working_directory", "description", "reason"]));

  return {
    title: description ?? (cwd ? `cwd: ${cwd}` : undefined),
    command,
    body: body && body !== command ? clampText(body, 4000) : undefined,
    params
  };
}

function extractCommand(value: unknown): string | undefined {
  const record = asRecord(value);
  if (!record) return undefined;
  const command = record.command ?? record.cmd;
  if (Array.isArray(command)) {
    return command.map((part) => shellPart(String(part))).join(" ");
  }
  if (typeof command === "string" && command.trim() !== "") {
    return command;
  }
  const nested = asRecord(record.input) ?? asRecord(record.arguments);
  if (nested) {
    return extractCommand(nested);
  }
  return undefined;
}

function extractString(value: unknown, ...keys: string[]): string | undefined {
  const record = asRecord(value);
  if (!record) return undefined;
  for (const key of keys) {
    const item = record[key];
    if (typeof item === "string" && item.trim() !== "") return item;
  }
  return undefined;
}

function previewParams(value: unknown, excluded: Set<string>): Array<[string, string]> {
  const record = asRecord(value);
  if (!record) return [];
  return Object.entries(record)
    .filter(([key]) => !excluded.has(key))
    .slice(0, 6)
    .map(([key, value]) => [key, compactValue(value)]);
}

function toolOutputText(value: unknown): string | undefined {
  if (typeof value === "string") return value;
  const record = asRecord(value);
  if (!record) return value === undefined || value === null ? undefined : compactValue(value);
  for (const key of ["output", "aggregated_output", "content", "result", "stdout", "stderr", "formatted_output"]) {
    const item = record[key];
    if (typeof item === "string" && item !== "") return item;
  }
  return formatJSON(value);
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

function compactValue(value: unknown): string {
  if (typeof value === "string") return clampText(value, 240).replace(/\s+/g, " ");
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (value == null) return "";
  return clampText(formatJSON(value), 240).replace(/\s+/g, " ");
}

function clampText(value: string, limit: number): string {
  if (value.length <= limit) return value;
  return `${value.slice(0, limit).trimEnd()}\n...`;
}

function shortID(value: string): string {
  return value.length > 18 ? `${value.slice(0, 10)}...${value.slice(-4)}` : value;
}

function shellPart(value: string): string {
  return /^[A-Za-z0-9_/:=.,@%+-]+$/.test(value) ? value : JSON.stringify(value);
}

function mergeToolProcessItems(items: ProcessItem[]): DisplayProcessItem[] {
  const merged: DisplayProcessItem[] = [];
  const toolCallIndexByID = new Map<string, number>();

  for (const item of items) {
    if (item.type === "tool_call") {
      merged.push({ ...item });
      if (item.tool_call_id) {
        toolCallIndexByID.set(item.tool_call_id, merged.length - 1);
      }
      continue;
    }

    if (item.type === "tool_result") {
      const matchingIndex = item.tool_call_id
        ? toolCallIndexByID.get(item.tool_call_id)
        : findPreviousOpenToolCallIndex(merged, item);

      if (matchingIndex !== undefined) {
        merged[matchingIndex] = mergeToolProcessPair(merged[matchingIndex], item);
        continue;
      }
    }

    merged.push({ ...item });
  }

  return merged;
}

function groupProcessFragments(items: DisplayProcessItem[]): DisplayProcessEntry[] {
  const grouped: DisplayProcessEntry[] = [];
  let tools: DisplayProcessItem[] = [];

  function flushTools() {
    if (tools.length === 0) {
      return;
    }
    grouped.push({ type: "tool_fragment", items: tools });
    tools = [];
  }

  for (const item of items) {
    if (item.type === "thinking") {
      flushTools();
      grouped.push(item);
      continue;
    }
    tools.push(item);
  }

  flushTools();
  return grouped;
}

function findPreviousOpenToolCallIndex(items: DisplayProcessItem[], result: ProcessItem): number | undefined {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const candidate = items[index];
    if (candidate.type !== "tool_call" || candidate.result) {
      continue;
    }
    if (candidate.tool_name && result.tool_name && candidate.tool_name !== result.tool_name) {
      continue;
    }
    return index;
  }
  return undefined;
}

function mergeToolProcessPair(call: DisplayProcessItem, result: ProcessItem): DisplayProcessItem {
  return {
    ...call,
    tool_name: call.tool_name ?? result.tool_name,
    tool_call_id: call.tool_call_id ?? result.tool_call_id,
    status: result.status ?? call.status,
    input: call.input ?? result.input,
    output: result.output ?? result.text ?? call.output,
    result
  };
}

function displayProcessItemFromDetail(detail: MessageProcessItemDetail): DisplayProcessItem {
  const item: DisplayProcessItem = { ...detail.item };
  if (!detail.result) {
    return item;
  }
  return mergeToolProcessPair(item, detail.result);
}

function processItemKey(item: DisplayProcessItem, index: number): string {
  if (item.tool_call_id) {
    return `tool-${item.tool_call_id}`;
  }
  if (item.result?.tool_call_id) {
    return `tool-${item.result.tool_call_id}`;
  }
  if (typeof item.process_index === "number") {
    return `${item.type}-${item.process_index}`;
  }
  return `${item.type}-${index}`;
}

function processEntryKey(item: DisplayProcessEntry, index: number): string {
  if (item.type === "tool_fragment") {
    const first = item.items[0];
    return `tool-fragment-${first ? processItemKey(first, index) : index}`;
  }
  return processItemKey(item, index);
}

function toolFragmentNames(items: DisplayProcessItem[]): string {
  const names = Array.from(
    new Set(items.map((item) => item.tool_name).filter((name): name is string => Boolean(name)))
  );
  if (names.length === 0) {
    return "";
  }
  if (names.length <= 3) {
    return names.join(", ");
  }
  return `${names.slice(0, 3).join(", ")} +${names.length - 3}`;
}

interface TextProcessSegment {
  text: string;
  processItems: ProcessItem[];
  subagentGroups: SubagentGroup[];
}

const processBreakPattern = /<!-- process-break:(\d+) -->/g;

function splitByProcessBreaks(
  text: string,
  mainItems: ProcessItem[],
  mainCounts: number[],
  subagentGroups: SubagentGroup[]
): TextProcessSegment[] | null {
  const matches = [...text.matchAll(processBreakPattern)];
  if (matches.length === 0) return null;

  // Break markers count every process item, subagent work included; translate
  // them into positions within the main agent's own items.
  const toMainCount = (n: number) => mainCounts[Math.max(0, Math.min(n, mainCounts.length - 1))];

  const segments: TextProcessSegment[] = [];
  let lastTextEnd = 0;
  let lastProcessEnd = 0;

  for (const match of matches) {
    const matchStart = match.index;
    const matchEnd = matchStart + match[0].length;
    const processCount = toMainCount(parseInt(match[1], 10));

    const rawText = text.slice(lastTextEnd, matchStart);
    const trimmed = rawText.replace(/^\n+|\n+$/g, "");
    segments.push({
      text: trimmed,
      processItems: mainItems.slice(lastProcessEnd, processCount),
      subagentGroups: subagentGroups.filter(
        (group) => group.anchor >= lastProcessEnd && group.anchor < processCount
      ),
    });

    lastTextEnd = matchEnd;
    lastProcessEnd = processCount;
  }

  const trailing = text.slice(lastTextEnd).replace(/^\n+|\n+$/g, "");
  segments.push({
    text: trailing,
    processItems: mainItems.slice(lastProcessEnd),
    subagentGroups: subagentGroups.filter((group) => group.anchor >= lastProcessEnd),
  });

  return segments;
}

export function InterleavedMessageBody({
  text,
  processItems,
  messageID,
  workspacePath,
  onOpenWorkspacePath,
  className,
  defaultProcessOpen = false,
  onStopSubagent,
}: {
  text: string;
  processItems: ProcessItem[];
  messageID?: string;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
  className?: string;
  defaultProcessOpen?: boolean;
  onStopSubagent?: (toolCallID: string) => Promise<void>;
}) {
  const { mainItems, subagentGroups, mainCounts } = partitionSubagentItems(processItems);
  const segments = splitByProcessBreaks(text, mainItems, mainCounts, subagentGroups);

  if (!segments) {
    return (
      <>
        {(mainItems.length > 0 || subagentGroups.length > 0) && (
          <ProcessBlock
            items={mainItems}
            subagentGroups={subagentGroups}
            defaultOpen={defaultProcessOpen}
            messageID={messageID}
            onStopSubagent={onStopSubagent}
          />
        )}
        {text.trim() !== "" && (
          <div className={className} data-testid="message-body">
            <MessageMarkdown text={text} workspacePath={workspacePath} onOpenWorkspacePath={onOpenWorkspacePath} />
          </div>
        )}
      </>
    );
  }

  return (
    <>
      {segments.map((segment, i) => (
        <Fragment key={i}>
          {segment.text.trim() !== "" && (
            <div className={className} data-testid="message-body">
              <MessageMarkdown
                text={segment.text}
                workspacePath={workspacePath}
                onOpenWorkspacePath={onOpenWorkspacePath}
              />
            </div>
          )}
          {(segment.processItems.length > 0 || segment.subagentGroups.length > 0) && (
            <ProcessBlock
              items={segment.processItems}
              subagentGroups={segment.subagentGroups}
              defaultOpen={defaultProcessOpen}
              messageID={messageID}
              onStopSubagent={onStopSubagent}
            />
          )}
        </Fragment>
      ))}
    </>
  );
}

export function processFromMetadata(metadata: Message["metadata"]): ProcessItem[] {
  if (metadata?.process && Array.isArray(metadata.process) && metadata.process.length > 0) {
    return metadata.process;
  }
  if (typeof metadata?.thinking === "string" && metadata.thinking.trim() !== "") {
    return [{ type: "thinking", text: metadata.thinking }];
  }
  return [];
}

export function processFromStreaming(item: StreamingMessage): ProcessItem[] {
  if (item.process && item.process.length > 0) {
    return item.process;
  }
  if (item.thinking && item.thinking.trim() !== "") {
    return [{ type: "thinking", text: item.thinking }];
  }
  return [];
}

interface SubagentInfo {
  toolCallID: string;
  description: string;
  active: boolean;
}

/** Codex spawns subagents via "Agent"; Claude Code via "Task". */
function isSubagentToolName(toolName?: string): boolean {
  return toolName === "Agent" || toolName === "Task";
}

/**
 * Whether the subagent behind a spawning call is still running. Task lifecycle
 * signals are authoritative when present — a background task's spawning call
 * resolves at launch, not at completion. Without signals (Codex, older
 * messages), an unresolved spawning call is the best available indicator.
 */
function subagentIsActive(
  toolCallID: string,
  startedIDs: Set<string>,
  finishedIDs: Set<string>,
  resolvedIDs: Set<string>
): boolean {
  if (startedIDs.has(toolCallID) || finishedIDs.has(toolCallID)) {
    return !finishedIDs.has(toolCallID);
  }
  return !resolvedIDs.has(toolCallID);
}

function collectSubagentSignals(process: ProcessItem[]): {
  startedIDs: Set<string>;
  finishedIDs: Set<string>;
  resolvedIDs: Set<string>;
} {
  const startedIDs = new Set<string>();
  const finishedIDs = new Set<string>();
  const resolvedIDs = new Set<string>();
  for (const item of process) {
    if (!item.tool_call_id) continue;
    if (item.type === "subagent_started") startedIDs.add(item.tool_call_id);
    if (item.type === "subagent_completed") finishedIDs.add(item.tool_call_id);
    if (item.type === "tool_result") resolvedIDs.add(item.tool_call_id);
  }
  return { startedIDs, finishedIDs, resolvedIDs };
}

export function getSubagentSummary(process: ProcessItem[]): SubagentInfo[] {
  const { startedIDs, finishedIDs, resolvedIDs } = collectSubagentSignals(process);

  const subagents: SubagentInfo[] = [];
  for (const item of process) {
    if (item.type === "tool_call" && isSubagentToolName(item.tool_name) && item.tool_call_id) {
      subagents.push({
        toolCallID: item.tool_call_id,
        description: subagentDescription(item),
        active: subagentIsActive(item.tool_call_id, startedIDs, finishedIDs, resolvedIDs),
      });
    }
  }
  return subagents;
}

export interface SubagentGroup {
  parentToolCallID: string;
  description: string;
  items: ProcessItem[];
  /** Index into mainItems where this group belongs: its spawning call, or where its work started. */
  anchor: number;
  /** Whether the subagent is still running. */
  active: boolean;
}

/**
 * Splits every subagent's process items away from the main agent's, so each
 * subagent's work can be shown under the call that spawned it instead of
 * flattened into the main timeline. Operates on the whole message at once —
 * process-break slicing happens afterwards over mainItems, using mainCounts to
 * translate break positions, so one subagent never splinters across blocks.
 */
export function partitionSubagentItems(items: ProcessItem[]): {
  mainItems: ProcessItem[];
  subagentGroups: SubagentGroup[];
  /** mainCounts[i] = how many of the first i process items belong to the main agent. */
  mainCounts: number[];
} {
  const mainItems: ProcessItem[] = [];
  const mainCounts: number[] = [0];
  const grouped = new Map<string, { items: ProcessItem[]; anchor: number }>();

  for (const item of items) {
    const parent = item.parent_tool_call_id;
    if (item.type === "subagent_started" || item.type === "subagent_completed") {
      // Pure state signals — consumed below for the active flag, never shown
      // as timeline rows. They still occupy a break-marker position.
    } else if (!parent) {
      mainItems.push(item);
    } else {
      const existing = grouped.get(parent);
      if (existing) {
        existing.items.push(item);
      } else {
        grouped.set(parent, { items: [item], anchor: mainItems.length });
      }
    }
    mainCounts.push(mainItems.length);
  }

  const { startedIDs, finishedIDs, resolvedIDs } = collectSubagentSignals(items);

  const spawningCalls = new Map<string, { description: string; anchor: number }>();
  mainItems.forEach((item, index) => {
    if (item.type === "tool_call" && item.tool_call_id && !spawningCalls.has(item.tool_call_id)) {
      spawningCalls.set(item.tool_call_id, { description: subagentDescription(item), anchor: index });
    }
  });

  const subagentGroups: SubagentGroup[] = [];
  for (const [parentToolCallID, group] of grouped) {
    const spawn = spawningCalls.get(parentToolCallID);
    subagentGroups.push({
      parentToolCallID,
      description: spawn?.description ?? "Subagent",
      items: group.items,
      anchor: spawn?.anchor ?? group.anchor,
      active: subagentIsActive(parentToolCallID, startedIDs, finishedIDs, resolvedIDs),
    });
  }
  return { mainItems, subagentGroups, mainCounts };
}

function subagentDescription(item: ProcessItem): string {
  const input = item.input as Record<string, unknown> | null;
  return truncateSubagentDescription(
    (typeof input?.description === "string" ? input.description : null) ??
    (typeof input?.subagent_type === "string" ? input.subagent_type : null) ??
    (typeof input?.prompt === "string" ? input.prompt : null) ??
    "Subagent"
  );
}

function truncateSubagentDescription(text: string, maxLen = 60): string {
  return text.length <= maxLen ? text : text.slice(0, maxLen - 1) + "…";
}

function rawProcessValue(item: DisplayProcessItem): unknown {
  if (item.result) {
    return {
      call: rawSingleProcessValue(item),
      result: rawSingleProcessValue(item.result)
    };
  }
  return rawSingleProcessValue(item);
}

function rawSingleProcessValue(item: ProcessItem): unknown {
  if (item.raw !== undefined) {
    return item.raw;
  }
  if (item.input !== undefined || item.output !== undefined) {
    return {
      ...(item.input !== undefined ? { input: item.input } : {}),
      ...(item.output !== undefined ? { output: item.output } : {})
    };
  }
  return undefined;
}

function formatJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return String(value);
  }
}

function useStreamingWorkingLabel(startedAt?: string, endedAt?: string): string | null {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    setNow(new Date());
  }, [startedAt, endedAt]);

  useEffect(() => {
    if (!startedAt || endedAt) {
      return;
    }
    const timer = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, [startedAt, endedAt]);

  return workingDurationBetween(startedAt, endedAt, now);
}
