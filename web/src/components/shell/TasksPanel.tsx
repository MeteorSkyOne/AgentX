import { useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { CalendarClock, CheckCircle2, Clock3, Pencil, Play, Plus, Trash2, XCircle } from "lucide-react";
import {
  createScheduledTask,
  deleteScheduledTask,
  runScheduledTask,
  scheduledTaskRuns,
  scheduledTasks,
  updateScheduledTask,
  type ScheduledTaskPayload,
} from "../../api/client";
import type {
  ActiveConversation,
} from "./types";
import type {
  Agent,
  Channel,
  ConversationType,
  Project,
  ScheduledTask,
  ScheduledTaskKind,
  ScheduledTaskRun,
  Thread,
  Workspace,
} from "../../api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

type TaskDraft = {
  name: string;
  kind: ScheduledTaskKind;
  enabled: boolean;
  schedule: string;
  timezone: string;
  conversationKey: string;
  agentID: string;
  workspaceID: string;
  prompt: string;
  command: string;
  timeoutSeconds: string;
};

type ConversationOption = {
  key: string;
  type: ConversationType;
  id: string;
  label: string;
};

export function TasksPanel({
  project,
  projectWorkspace,
  channels,
  threads,
  activeConversation,
  agents,
}: {
  project?: Project;
  projectWorkspace?: Workspace;
  channels: Channel[];
  threads: Thread[];
  activeConversation?: ActiveConversation;
  agents: Agent[];
}) {
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<ScheduledTask | null>(null);
  const [selectedTaskID, setSelectedTaskID] = useState("");
  const [draft, setDraft] = useState<TaskDraft>(() => blankDraft(projectWorkspace?.id));
  const [actionError, setActionError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const tasksQuery = useQuery({
    queryKey: ["scheduled-tasks", project?.id],
    queryFn: () => scheduledTasks(project!.id),
    enabled: Boolean(project?.id),
    refetchInterval: 5000,
  });
  const tasks = tasksQuery.data ?? [];
  const selectedTask = tasks.find((task) => task.id === selectedTaskID) ?? tasks[0];
  const runsQuery = useQuery({
    queryKey: ["scheduled-task-runs", selectedTask?.id],
    queryFn: () => scheduledTaskRuns(selectedTask!.id),
    enabled: Boolean(selectedTask?.id),
    refetchInterval: selectedTask ? 5000 : false,
  });
  const conversationOptions = useMemo(
    () => buildConversationOptions(channels, threads, activeConversation),
    [channels, threads, activeConversation]
  );
  const enabledAgents = useMemo(() => agents.filter((agent) => agent.enabled), [agents]);

  function openCreateDialog() {
    setEditingTask(null);
    setDraft(blankDraft(projectWorkspace?.id, defaultConversationKey(conversationOptions)));
    setActionError(null);
    setDialogOpen(true);
  }

  function openEditDialog(task: ScheduledTask) {
    setEditingTask(task);
    setDraft(draftFromTask(task, projectWorkspace?.id, defaultConversationKey(conversationOptions)));
    setActionError(null);
    setDialogOpen(true);
  }

  async function saveTask() {
    if (!project) return;
    setPending(true);
    setActionError(null);
    try {
      const payload = payloadFromDraft(draft);
      const saved = editingTask
        ? await updateScheduledTask(editingTask.id, payload)
        : await createScheduledTask(project.id, payload);
      setSelectedTaskID(saved.id);
      setDialogOpen(false);
      await invalidateTasks(queryClient, project.id, saved.id);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Save task failed");
    } finally {
      setPending(false);
    }
  }

  async function toggleTask(task: ScheduledTask) {
    if (!project) return;
    setPending(true);
    setActionError(null);
    try {
      await updateScheduledTask(task.id, { enabled: !task.enabled });
      await invalidateTasks(queryClient, project.id, task.id);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Update task failed");
    } finally {
      setPending(false);
    }
  }

  async function runNow(task: ScheduledTask) {
    if (!project) return;
    setPending(true);
    setActionError(null);
    try {
      await runScheduledTask(task.id);
      setSelectedTaskID(task.id);
      await invalidateTasks(queryClient, project.id, task.id);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Run task failed");
    } finally {
      setPending(false);
    }
  }

  async function removeTask(task: ScheduledTask) {
    if (!project || !window.confirm(`Delete "${task.name}"?`)) return;
    setPending(true);
    setActionError(null);
    try {
      await deleteScheduledTask(task.id);
      setSelectedTaskID("");
      await queryClient.invalidateQueries({ queryKey: ["scheduled-tasks", project.id] });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Delete task failed");
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="flex min-h-0 flex-1 flex-col bg-background" data-testid="tasks-panel">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
        <div className="flex min-w-0 items-center gap-2">
          <CalendarClock className="h-4 w-4 text-muted-foreground" />
          <span className="truncate text-sm font-semibold">Tasks</span>
        </div>
        <Button size="sm" className="h-8 gap-1" disabled={!project} onClick={openCreateDialog}>
          <Plus className="h-4 w-4" />
          New
        </Button>
      </div>

      {actionError && (
        <div className="mx-4 mt-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {actionError}
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[minmax(0,1fr)_24rem]">
        <ScrollArea className="min-h-0 border-r border-border">
          <div className="space-y-2 p-3">
            {tasksQuery.isLoading ? (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">Loading tasks...</div>
            ) : tasks.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">No tasks yet</div>
            ) : (
              tasks.map((task) => (
                <TaskRow
                  key={task.id}
                  task={task}
                  selected={task.id === selectedTask?.id}
                  pending={pending}
                  onSelect={() => setSelectedTaskID(task.id)}
                  onEdit={() => openEditDialog(task)}
                  onToggle={() => void toggleTask(task)}
                  onRun={() => void runNow(task)}
                  onDelete={() => void removeTask(task)}
                />
              ))
            )}
          </div>
        </ScrollArea>

        <RunHistory task={selectedTask} runs={runsQuery.data ?? []} loading={runsQuery.isLoading} />
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingTask ? "Edit task" : "New task"}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="task-name">Name</Label>
              <Input
                id="task-name"
                value={draft.name}
                onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
                autoFocus
              />
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="grid gap-2">
                <Label htmlFor="task-kind">Type</Label>
                <Select
                  id="task-kind"
                  value={draft.kind}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, kind: event.target.value as ScheduledTaskKind }))
                  }
                >
                  <option value="agent_prompt">Agent prompt</option>
                  <option value="shell_command">Shell command</option>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="task-schedule">Schedule</Label>
                <Input
                  id="task-schedule"
                  value={draft.schedule}
                  onChange={(event) => setDraft((current) => ({ ...current, schedule: event.target.value }))}
                  placeholder="0 9 * * *"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="task-timezone">Timezone</Label>
                <Input
                  id="task-timezone"
                  value={draft.timezone}
                  onChange={(event) => setDraft((current) => ({ ...current, timezone: event.target.value }))}
                  placeholder="UTC"
                />
              </div>
            </div>

            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={draft.enabled}
                onChange={(event) => setDraft((current) => ({ ...current, enabled: event.target.checked }))}
              />
              Enabled
            </label>

            {draft.kind === "agent_prompt" ? (
              <>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="task-conversation">Conversation</Label>
                    <Select
                      id="task-conversation"
                      value={draft.conversationKey}
                      onChange={(event) =>
                        setDraft((current) => ({ ...current, conversationKey: event.target.value }))
                      }
                    >
                      {conversationOptions.map((option) => (
                        <option key={option.key} value={option.key}>
                          {option.label}
                        </option>
                      ))}
                    </Select>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="task-agent">Agent</Label>
                    <Select
                      id="task-agent"
                      value={draft.agentID}
                      onChange={(event) => setDraft((current) => ({ ...current, agentID: event.target.value }))}
                    >
                      <option value="">All bound agents</option>
                      {enabledAgents.map((agent) => (
                        <option key={agent.id} value={agent.id}>
                          {agent.name}
                        </option>
                      ))}
                    </Select>
                  </div>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="task-prompt">Prompt</Label>
                  <Textarea
                    id="task-prompt"
                    rows={7}
                    value={draft.prompt}
                    onChange={(event) => setDraft((current) => ({ ...current, prompt: event.target.value }))}
                  />
                </div>
              </>
            ) : (
              <>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_10rem]">
                  <div className="grid gap-2">
                    <Label htmlFor="task-workspace">Workspace ID</Label>
                    <Input
                      id="task-workspace"
                      value={draft.workspaceID}
                      onChange={(event) => setDraft((current) => ({ ...current, workspaceID: event.target.value }))}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="task-timeout">Timeout</Label>
                    <Input
                      id="task-timeout"
                      type="number"
                      min={1}
                      max={86400}
                      value={draft.timeoutSeconds}
                      onChange={(event) =>
                        setDraft((current) => ({ ...current, timeoutSeconds: event.target.value }))
                      }
                    />
                  </div>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="task-command">Command</Label>
                  <Textarea
                    id="task-command"
                    rows={7}
                    value={draft.command}
                    onChange={(event) => setDraft((current) => ({ ...current, command: event.target.value }))}
                  />
                </div>
              </>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={pending}>
              Cancel
            </Button>
            <Button onClick={() => void saveTask()} disabled={pending || !draft.name.trim() || !draft.schedule.trim()}>
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

function TaskRow({
  task,
  selected,
  pending,
  onSelect,
  onEdit,
  onToggle,
  onRun,
  onDelete,
}: {
  task: ScheduledTask;
  selected: boolean;
  pending: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onToggle: () => void;
  onRun: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      className={cn(
        "w-full cursor-pointer rounded-md border border-border bg-card p-3 text-left transition-colors hover:bg-accent/40 focus-visible:ring-[3px] focus-visible:ring-ring/30 focus-visible:outline-none",
        selected && "border-ring bg-accent/50"
      )}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onSelect();
        }
      }}
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-sm font-medium">{task.name}</span>
            <Badge variant={task.enabled ? "secondary" : "outline"}>{task.enabled ? "Enabled" : "Paused"}</Badge>
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>{task.kind === "agent_prompt" ? "Agent prompt" : "Shell command"}</span>
            <span>{task.schedule}</span>
            <span>{task.timezone}</span>
          </div>
          <div className="mt-2 text-xs text-muted-foreground">
            {task.next_run_at ? `Next ${formatDateTime(task.next_run_at)}` : "No scheduled run"}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1" onClick={(event) => event.stopPropagation()}>
          <IconButton title="Run now" disabled={pending} onClick={onRun}>
            <Play className="h-4 w-4" />
          </IconButton>
          <IconButton title={task.enabled ? "Pause" : "Enable"} disabled={pending} onClick={onToggle}>
            <Clock3 className="h-4 w-4" />
          </IconButton>
          <IconButton title="Edit" disabled={pending} onClick={onEdit}>
            <Pencil className="h-4 w-4" />
          </IconButton>
          <IconButton title="Delete" disabled={pending} onClick={onDelete}>
            <Trash2 className="h-4 w-4" />
          </IconButton>
        </div>
      </div>
      {task.last_run_status && (
        <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
          <RunStatusIcon status={task.last_run_status} />
          <span>{task.last_run_status}</span>
          {task.last_run_at && <span>{formatDateTime(task.last_run_at)}</span>}
        </div>
      )}
    </div>
  );
}

function RunHistory({
  task,
  runs,
  loading,
}: {
  task?: ScheduledTask;
  runs: ScheduledTaskRun[];
  loading: boolean;
}) {
  return (
    <aside className="min-h-0 bg-muted/10">
      <div className="flex h-12 items-center border-b border-border px-4 text-sm font-semibold">Runs</div>
      <ScrollArea className="h-[calc(100%-3rem)]">
        <div className="space-y-3 p-3">
          {!task ? (
            <div className="py-8 text-center text-sm text-muted-foreground">Select a task</div>
          ) : loading ? (
            <div className="py-8 text-center text-sm text-muted-foreground">Loading runs...</div>
          ) : runs.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">No runs yet</div>
          ) : (
            runs.map((run) => <RunRow key={run.id} run={run} />)
          )}
        </div>
      </ScrollArea>
    </aside>
  );
}

function RunRow({ run }: { run: ScheduledTaskRun }) {
  return (
    <div className="rounded-md border border-border bg-card p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <RunStatusIcon status={run.status} />
          <span className="truncate text-sm font-medium">{run.status}</span>
        </div>
        <Badge variant="outline">{run.trigger}</Badge>
      </div>
      <div className="mt-2 text-xs text-muted-foreground">{formatDateTime(run.started_at)}</div>
      {run.error && <div className="mt-2 text-xs text-destructive">{run.error}</div>}
      {run.stdout && <OutputBlock label="stdout" value={run.stdout} />}
      {run.stderr && <OutputBlock label="stderr" value={run.stderr} />}
      {run.output_truncated && <div className="mt-2 text-xs text-muted-foreground">output truncated</div>}
    </div>
  );
}

function OutputBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="mt-3">
      <div className="mb-1 text-[11px] uppercase text-muted-foreground">{label}</div>
      <pre className="max-h-40 overflow-auto rounded-md bg-muted p-2 text-xs leading-5 text-foreground">
        {value}
      </pre>
    </div>
  );
}

function IconButton({
  title,
  disabled,
  onClick,
  children,
}: {
  title: string;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Button
      variant="ghost"
      size="icon"
      className="h-8 w-8"
      title={title}
      aria-label={title}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function RunStatusIcon({ status }: { status: string }) {
  if (status === "completed") return <CheckCircle2 className="h-4 w-4 text-emerald-500" />;
  if (status === "failed") return <XCircle className="h-4 w-4 text-destructive" />;
  return <Clock3 className="h-4 w-4 text-muted-foreground" />;
}

function blankDraft(workspaceID = "", conversationKey = ""): TaskDraft {
  return {
    name: "",
    kind: "agent_prompt",
    enabled: true,
    schedule: "0 9 * * *",
    timezone: "UTC",
    conversationKey,
    agentID: "",
    workspaceID,
    prompt: "",
    command: "",
    timeoutSeconds: "600",
  };
}

function draftFromTask(task: ScheduledTask, workspaceID = "", fallbackConversationKey = ""): TaskDraft {
  const conversationKey =
    task.conversation_type && task.conversation_id
      ? conversationKeyFor(task.conversation_type, task.conversation_id)
      : fallbackConversationKey;
  return {
    name: task.name,
    kind: task.kind,
    enabled: task.enabled,
    schedule: task.schedule,
    timezone: task.timezone || "UTC",
    conversationKey,
    agentID: task.agent_id ?? "",
    workspaceID: task.workspace_id ?? workspaceID,
    prompt: task.prompt ?? "",
    command: task.command ?? "",
    timeoutSeconds: String(task.timeout_seconds || 600),
  };
}

function payloadFromDraft(draft: TaskDraft): ScheduledTaskPayload {
  const conversation = parseConversationKey(draft.conversationKey);
  return {
    name: draft.name.trim(),
    kind: draft.kind,
    enabled: draft.enabled,
    schedule: draft.schedule.trim(),
    timezone: draft.timezone.trim() || "UTC",
    conversation_type: draft.kind === "agent_prompt" ? conversation?.type ?? "" : "",
    conversation_id: draft.kind === "agent_prompt" ? conversation?.id ?? "" : "",
    agent_id: draft.kind === "agent_prompt" ? draft.agentID : "",
    workspace_id: draft.workspaceID.trim(),
    prompt: draft.kind === "agent_prompt" ? draft.prompt : "",
    command: draft.kind === "shell_command" ? draft.command : "",
    timeout_seconds: Number(draft.timeoutSeconds) || 600,
  };
}

function buildConversationOptions(
  channels: Channel[],
  threads: Thread[],
  activeConversation?: ActiveConversation
): ConversationOption[] {
  const options = channels
    .filter((channel) => channel.type === "text")
    .map((channel) => ({
      key: conversationKeyFor("channel", channel.id),
      type: "channel" as ConversationType,
      id: channel.id,
      label: `#${channel.name}`,
    }));
  for (const thread of threads) {
    options.push({
      key: conversationKeyFor("thread", thread.id),
      type: "thread",
      id: thread.id,
      label: thread.title,
    });
  }
  if (activeConversation && !options.some((option) => option.key === conversationKeyFor(activeConversation.type, activeConversation.id))) {
    options.unshift({
      key: conversationKeyFor(activeConversation.type, activeConversation.id),
      type: activeConversation.type,
      id: activeConversation.id,
      label: activeConversation.type === "channel" ? "Current channel" : "Current thread",
    });
  }
  return options;
}

function defaultConversationKey(options: ConversationOption[]): string {
  return options[0]?.key ?? "";
}

function conversationKeyFor(type: ConversationType, id: string): string {
  return `${type}:${id}`;
}

function parseConversationKey(value: string): { type: ConversationType; id: string } | null {
  const [type, id] = value.split(":", 2);
  if ((type === "channel" || type === "thread" || type === "dm") && id) {
    return { type, id };
  }
  return null;
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

async function invalidateTasks(queryClient: QueryClient, projectID: string, taskID: string) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["scheduled-tasks", projectID] }),
    queryClient.invalidateQueries({ queryKey: ["scheduled-task-runs", taskID] }),
  ]);
}
