import { useEffect, useRef, useState } from "react";
import { skipToken, useQuery } from "@tanstack/react-query";
import { Activity, Bot, ChevronDown, ChevronRight, FileText, Loader2, Square, SquareTerminal } from "lucide-react";
import { fetchBackgroundTaskOutput, listBackgroundTasks, stopBackgroundTask } from "../api/client";
import type { Agent, BackgroundTask, ConversationType } from "../api/types";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** Ended tasks stay listed this long after they finish. */
const RECENT_ENDED_MS = 30 * 60 * 1000;
const OUTPUT_REFRESH_MS = 2000;

export function backgroundTasksQueryKey(type: ConversationType | undefined, id: string | undefined) {
  return ["background-tasks", type, id] as const;
}

/** Replaces one agent's tasks in a conversation's task list, oldest first. */
export function mergeAgentBackgroundTasks(
  current: BackgroundTask[] | undefined,
  agentID: string,
  tasks: BackgroundTask[]
): BackgroundTask[] {
  return [...(current ?? []).filter((task) => task.agent_id !== agentID), ...tasks].sort(
    (a, b) => Date.parse(a.started_at) - Date.parse(b.started_at)
  );
}

/** Tasks worth showing: running ones and those that ended recently. */
export function visibleBackgroundTasks(tasks: BackgroundTask[], now: number): BackgroundTask[] {
  return tasks.filter(
    (task) => task.status === "running" || !task.ended_at || now - Date.parse(task.ended_at) < RECENT_ENDED_MS
  );
}

export function formatTaskDuration(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, "0")}m`;
}

const kindMeta: Record<string, { label: string; Icon: typeof Bot }> = {
  shell: { label: "Shell", Icon: SquareTerminal },
  monitor: { label: "Monitor", Icon: Activity },
  agent: { label: "Subagent", Icon: Bot }
};

function statusLabel(status: string): string {
  switch (status) {
    case "running":
      return "Running";
    case "completed":
      return "Done";
    case "failed":
    case "error":
      return "Failed";
    case "stopped":
    case "killed":
    case "cancelled":
    case "canceled":
      return "Stopped";
    default:
      return status;
  }
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);
  return now;
}

interface BackgroundTasksProps {
  conversation?: { type: ConversationType; id: string };
  agents?: Pick<Agent, "id" | "name">[];
}

/**
 * Lists the shell commands, monitors and subagents agents left running in the
 * conversation, with their output and a way to stop them. The list is kept
 * current by AgentBackgroundTasksUpdated events written into the query cache.
 */
export function BackgroundTasks({ conversation, agents = [] }: BackgroundTasksProps) {
  const [expanded, setExpanded] = useState(false);
  const tasksQuery = useQuery({
    queryKey: backgroundTasksQueryKey(conversation?.type, conversation?.id),
    queryFn: conversation ? () => listBackgroundTasks(conversation.type, conversation.id) : skipToken,
    staleTime: 30_000
  });
  const tasks = tasksQuery.data ?? [];
  const anyRunning = tasks.some((task) => task.status === "running");
  const now = useNow(anyRunning);
  const visible = visibleBackgroundTasks(tasks, now);
  if (!conversation || visible.length === 0) {
    return null;
  }
  const running = visible.filter((task) => task.status === "running").length;
  const finished = visible.length - running;

  return (
    <div className="space-y-1">
      <button
        type="button"
        className="flex items-center gap-1 text-left"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
      >
        {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        <span className="font-medium text-foreground">Background tasks</span>
        {running > 0 && (
          <span className="flex items-center gap-1">
            <Loader2 className="h-3 w-3 animate-spin" />
            {running} running
          </span>
        )}
        {running > 0 && finished > 0 && <span aria-hidden>·</span>}
        {finished > 0 && <span>{finished} finished</span>}
      </button>
      {expanded && (
        <div className="flex flex-col gap-1">
          {visible.map((task) => (
            <BackgroundTaskRow
              key={`${task.agent_id}:${task.id}`}
              conversation={conversation}
              task={task}
              agentName={agents.length > 1 ? agents.find((agent) => agent.id === task.agent_id)?.name : undefined}
              now={now}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function BackgroundTaskRow({
  conversation,
  task,
  agentName,
  now
}: {
  conversation: { type: ConversationType; id: string };
  task: BackgroundTask;
  agentName?: string;
  now: number;
}) {
  const [showOutput, setShowOutput] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const running = task.status === "running";
  const { label, Icon } = kindMeta[task.kind] ?? { label: task.kind, Icon: SquareTerminal };
  const end = task.ended_at ? Date.parse(task.ended_at) : now;
  const duration = formatTaskDuration(end - Date.parse(task.started_at));

  async function handleStop() {
    setStopping(true);
    setError(null);
    try {
      await stopBackgroundTask(conversation.type, conversation.id, task.agent_id, task.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Stop failed");
    } finally {
      setStopping(false);
    }
  }

  return (
    <div className={cn("rounded-md border border-border bg-muted/35 px-2 py-1", !running && "opacity-75")}>
      <div className="flex min-h-6 items-center gap-2">
        <Icon className="h-3.5 w-3.5 shrink-0" aria-label={label} />
        {agentName && <span className="shrink-0 font-medium text-foreground">{agentName}</span>}
        <span className="min-w-0 flex-1 truncate" title={task.description}>
          {task.description || label}
        </span>
        <span
          className={cn(
            "shrink-0",
            running && "text-foreground",
            statusLabel(task.status) === "Failed" && "text-destructive"
          )}
        >
          {statusLabel(task.status)} · {duration}
        </span>
        {task.has_output && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0"
            title={showOutput ? "Hide output" : "Show output"}
            aria-label={showOutput ? "Hide output" : "Show output"}
            aria-pressed={showOutput}
            onClick={() => setShowOutput((current) => !current)}
          >
            <FileText className="h-3.5 w-3.5" />
          </Button>
        )}
        {running && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0 text-muted-foreground hover:text-destructive"
            title="Stop task"
            aria-label="Stop task"
            disabled={stopping}
            onClick={() => void handleStop()}
          >
            <Square className="h-3 w-3" />
          </Button>
        )}
      </div>
      {task.summary && !running && <div className="pb-1 pl-5.5">{task.summary}</div>}
      {error && <div className="pb-1 pl-5.5 text-destructive">{error}</div>}
      {showOutput && <BackgroundTaskOutputView conversation={conversation} task={task} />}
    </div>
  );
}

function BackgroundTaskOutputView({
  conversation,
  task
}: {
  conversation: { type: ConversationType; id: string };
  task: BackgroundTask;
}) {
  const running = task.status === "running";
  const outputQuery = useQuery({
    queryKey: ["background-task-output", conversation.type, conversation.id, task.agent_id, task.id],
    queryFn: () => fetchBackgroundTaskOutput(conversation.type, conversation.id, task.agent_id, task.id),
    refetchInterval: running ? OUTPUT_REFRESH_MS : false
  });
  // Follow the newest output like a terminal, unless the user scrolled up.
  const preRef = useRef<HTMLPreElement>(null);
  const followRef = useRef(true);
  const output = outputQuery.data?.output;
  useEffect(() => {
    const pre = preRef.current;
    if (pre && followRef.current) {
      pre.scrollTop = pre.scrollHeight;
    }
  }, [output]);

  if (outputQuery.isPending) {
    return <div className="py-1">Loading output…</div>;
  }
  if (outputQuery.isError) {
    return <div className="py-1 text-destructive">Could not load output.</div>;
  }
  return (
    <div className="pb-1">
      {outputQuery.data.truncated && <div className="py-0.5 italic">Earlier output omitted.</div>}
      <pre
        ref={preRef}
        onScroll={(event) => {
          const pre = event.currentTarget;
          followRef.current = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 16;
        }}
        className="max-h-48 overflow-auto whitespace-pre-wrap break-all rounded border border-border bg-background p-2 font-mono text-[11px] leading-snug text-foreground">
        {output || (running ? "No output yet." : "No output.")}
      </pre>
    </div>
  );
}
