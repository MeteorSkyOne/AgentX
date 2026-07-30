import { useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import {
  CalendarClock,
  CheckCircle2,
  Clock3,
  ExternalLink,
  History,
  Pencil,
  Play,
  Plus,
  Trash2,
  XCircle,
} from "lucide-react";
import {
  channelThreads,
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
  DialogDescription,
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

const RUN_HISTORY_LIMIT = 100;

type TaskDraft = {
  name: string;
  kind: ScheduledTaskKind;
  enabled: boolean;
  schedule: string;
  timezone: string;
  conversationKey: string;
  forumChannelID: string;
  postTitle: string;
  freshContext: boolean;
  notify: boolean;
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
  onOpenChannel,
  onOpenThread,
}: {
  project?: Project;
  projectWorkspace?: Workspace;
  channels: Channel[];
  threads: Thread[];
  activeConversation?: ActiveConversation;
  agents: Agent[];
  onOpenChannel?: (channel: Channel) => void;
  onOpenThread?: (thread: Thread) => void;
}) {
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<ScheduledTask | null>(null);
  const [selectedTaskID, setSelectedTaskID] = useState("");
  const [historyTaskID, setHistoryTaskID] = useState("");
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
  const historyTask = tasks.find((task) => task.id === historyTaskID);
  const runsQuery = useQuery({
    queryKey: ["scheduled-task-runs", selectedTask?.id],
    queryFn: () => scheduledTaskRuns(selectedTask!.id),
    enabled: Boolean(selectedTask?.id),
    refetchInterval: selectedTask ? 5000 : false,
  });
  const historyQuery = useQuery({
    queryKey: ["scheduled-task-runs", historyTask?.id, RUN_HISTORY_LIMIT],
    queryFn: () => scheduledTaskRuns(historyTask!.id, RUN_HISTORY_LIMIT),
    enabled: Boolean(historyTask?.id),
    refetchInterval: historyTask ? 5000 : false,
  });
  const conversationOptions = useMemo(
    () => buildConversationOptions(channels, threads, activeConversation),
    [channels, threads, activeConversation]
  );
  const forumChannels = useMemo(() => channels.filter((channel) => channel.type === "thread"), [channels]);
  const enabledAgents = useMemo(() => agents.filter((agent) => agent.enabled), [agents]);

  function openCreateDialog() {
    setEditingTask(null);
    setDraft(
      blankDraft(projectWorkspace?.id, defaultConversationKey(conversationOptions), forumChannels[0]?.id)
    );
    setActionError(null);
    setDialogOpen(true);
  }

  function openEditDialog(task: ScheduledTask) {
    setEditingTask(task);
    setDraft(
      draftFromTask(
        task,
        projectWorkspace?.id,
        defaultConversationKey(conversationOptions),
        forumChannels[0]?.id
      )
    );
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

  async function openRunTarget(task: ScheduledTask, run: ScheduledTaskRun) {
    setActionError(null);
    try {
      if (run.thread_id) {
        const channelID = task.kind === "forum_post" ? task.conversation_id : undefined;
        const thread = await resolveThread(queryClient, run.thread_id, channelID, threads);
        if (thread) onOpenThread?.(thread);
        return;
      }
      if (task.conversation_type === "thread" && task.conversation_id) {
        const thread = await resolveThread(queryClient, task.conversation_id, undefined, threads);
        if (thread) onOpenThread?.(thread);
        return;
      }
      if (task.conversation_type === "channel" && task.conversation_id) {
        const channel = channels.find((item) => item.id === task.conversation_id);
        if (channel) onOpenChannel?.(channel);
      }
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Open run target failed");
    }
  }

  // Only offer navigation when the target can actually be resolved: forum posts
  // load threads from their forum channel, everything else must already be known.
  function runTargetLabel(task: ScheduledTask, run: ScheduledTaskRun): string | undefined {
    if (run.thread_id) {
      if (task.kind === "forum_post" && task.conversation_id) return "Open post";
      return threads.some((thread) => thread.id === run.thread_id) ? "Open post" : undefined;
    }
    if (task.conversation_type === "thread") {
      return threads.some((thread) => thread.id === task.conversation_id) ? "Open thread" : undefined;
    }
    if (task.conversation_type === "channel") {
      return channels.some((channel) => channel.id === task.conversation_id) ? "Open channel" : undefined;
    }
    return undefined;
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
                  onHistory={() => setHistoryTaskID(task.id)}
                  onDelete={() => void removeTask(task)}
                />
              ))
            )}
          </div>
        </ScrollArea>

        <RunHistory
          task={selectedTask}
          runs={runsQuery.data ?? []}
          loading={runsQuery.isLoading}
          onShowAll={selectedTask ? () => setHistoryTaskID(selectedTask.id) : undefined}
          onOpenTarget={openRunTarget}
          runTargetLabel={runTargetLabel}
        />
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
                  <option value="forum_post">Forum post</option>
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

            {draft.kind === "shell_command" ? (
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
            ) : (
              <>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {draft.kind === "forum_post" ? (
                    <div className="grid gap-2">
                      <Label htmlFor="task-forum">Forum</Label>
                      <Select
                        id="task-forum"
                        value={draft.forumChannelID}
                        onChange={(event) =>
                          setDraft((current) => ({ ...current, forumChannelID: event.target.value }))
                        }
                      >
                        {forumChannels.length === 0 && <option value="">No forum channels</option>}
                        {forumChannels.map((channel) => (
                          <option key={channel.id} value={channel.id}>
                            #{channel.name}
                          </option>
                        ))}
                      </Select>
                    </div>
                  ) : (
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
                  )}
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

                {draft.kind === "forum_post" ? (
                  <div className="grid gap-2">
                    <Label htmlFor="task-post-title">Post title</Label>
                    <Input
                      id="task-post-title"
                      value={draft.postTitle}
                      onChange={(event) => setDraft((current) => ({ ...current, postTitle: event.target.value }))}
                      placeholder="${task} · ${datetime}"
                    />
                    <p className="text-xs text-muted-foreground">
                      Placeholders: {"${task}"}, {"${date}"}, {"${time}"}, {"${datetime}"}. Each run opens a new
                      post, so it always starts from an empty conversation.
                    </p>
                  </div>
                ) : (
                  <label className="flex items-start gap-2 text-sm">
                    <Checkbox
                      className="mt-0.5"
                      checked={draft.freshContext}
                      onChange={(event) =>
                        setDraft((current) => ({ ...current, freshContext: event.target.checked }))
                      }
                    />
                    <span>
                      Fresh context
                      <span className="block text-xs text-muted-foreground">
                        Reset the agent session before each run, so no conversation history is sent.
                      </span>
                    </span>
                  </label>
                )}

                <label className="flex items-start gap-2 text-sm">
                  <Checkbox
                    className="mt-0.5"
                    checked={draft.notify}
                    onChange={(event) => setDraft((current) => ({ ...current, notify: event.target.checked }))}
                  />
                  <span>
                    Notify on reply
                    <span className="block text-xs text-muted-foreground">
                      Send webhook and browser notifications when this task's agents reply. Turn it off for
                      routine upkeep runs.
                    </span>
                  </span>
                </label>

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
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={pending}>
              Cancel
            </Button>
            <Button
              onClick={() => void saveTask()}
              disabled={
                pending ||
                !draft.name.trim() ||
                !draft.schedule.trim() ||
                (draft.kind === "forum_post" && !draft.forumChannelID)
              }
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(historyTask)} onOpenChange={(open) => !open && setHistoryTaskID("")}>
        <DialogContent className="max-h-[92vh] overflow-hidden sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>Run history</DialogTitle>
            <DialogDescription>
              {historyTask ? `${historyTask.name} · last ${RUN_HISTORY_LIMIT} runs` : ""}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className="max-h-[65vh]">
            <div className="space-y-3 pr-3">
              {historyQuery.isLoading ? (
                <div className="py-8 text-center text-sm text-muted-foreground">Loading runs...</div>
              ) : (historyQuery.data ?? []).length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No runs yet</div>
              ) : (
                (historyQuery.data ?? []).map((run) => (
                  <RunRow
                    key={run.id}
                    run={run}
                    detailed
                    openLabel={historyTask ? runTargetLabel(historyTask, run) : undefined}
                    onOpenTarget={
                      historyTask
                        ? () => {
                            setHistoryTaskID("");
                            void openRunTarget(historyTask, run);
                          }
                        : undefined
                    }
                  />
                ))
              )}
            </div>
          </ScrollArea>
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
  onHistory,
  onDelete,
}: {
  task: ScheduledTask;
  selected: boolean;
  pending: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onToggle: () => void;
  onRun: () => void;
  onHistory: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      className={cn(
        "w-full cursor-pointer rounded-xl border-2 border-border bg-card p-3 text-left shadow-chunk-xs transition-all hover:bg-accent/40 hover:shadow-chunk-sm focus-visible:ring-[3px] focus-visible:ring-ring/30 focus-visible:outline-none",
        selected && "border-ring bg-accent/50 shadow-chunk-sm"
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
            <span>{taskKindLabel(task.kind)}</span>
            <span>{task.schedule}</span>
            <span>{task.timezone}</span>
            {task.fresh_context && <span>fresh context</span>}
            {task.kind !== "shell_command" && !task.notify && <span>muted</span>}
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
          <IconButton title="Run history" onClick={onHistory}>
            <History className="h-4 w-4" />
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
  onShowAll,
  onOpenTarget,
  runTargetLabel,
}: {
  task?: ScheduledTask;
  runs: ScheduledTaskRun[];
  loading: boolean;
  onShowAll?: () => void;
  onOpenTarget: (task: ScheduledTask, run: ScheduledTaskRun) => void | Promise<void>;
  runTargetLabel: (task: ScheduledTask, run: ScheduledTaskRun) => string | undefined;
}) {
  return (
    <aside className="min-h-0 bg-muted/10">
      <div className="flex h-12 items-center justify-between border-b border-border px-4 text-sm font-semibold">
        <span>Runs</span>
        {onShowAll && (
          <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs font-medium" onClick={onShowAll}>
            <History className="h-3.5 w-3.5" />
            View all
          </Button>
        )}
      </div>
      <ScrollArea className="h-[calc(100%-3rem)]">
        <div className="space-y-3 p-3">
          {!task ? (
            <div className="py-8 text-center text-sm text-muted-foreground">Select a task</div>
          ) : loading ? (
            <div className="py-8 text-center text-sm text-muted-foreground">Loading runs...</div>
          ) : runs.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">No runs yet</div>
          ) : (
            runs.map((run) => (
              <RunRow
                key={run.id}
                run={run}
                openLabel={runTargetLabel(task, run)}
                onOpenTarget={() => void onOpenTarget(task, run)}
              />
            ))
          )}
        </div>
      </ScrollArea>
    </aside>
  );
}

function RunRow({
  run,
  detailed,
  openLabel,
  onOpenTarget,
}: {
  run: ScheduledTaskRun;
  detailed?: boolean;
  openLabel?: string;
  onOpenTarget?: () => void;
}) {
  const duration = formatDuration(run.started_at, run.finished_at);
  return (
    <div className="rounded-xl border-2 border-border bg-card p-3 shadow-chunk-xs">
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <RunStatusIcon status={run.status} />
          <span className="truncate text-sm font-medium">{run.status}</span>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Badge variant="outline">{run.trigger}</Badge>
          {openLabel && onOpenTarget && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 gap-1 text-xs font-medium"
              title={openLabel}
              onClick={onOpenTarget}
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {openLabel}
            </Button>
          )}
        </div>
      </div>
      <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span>{formatDateTime(run.started_at)}</span>
        {duration && <span>{duration}</span>}
        {detailed && run.scheduled_for && <span>due {formatDateTime(run.scheduled_for)}</span>}
        {typeof run.exit_code === "number" && <span>exit {run.exit_code}</span>}
      </div>
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

function taskKindLabel(kind: ScheduledTaskKind): string {
  if (kind === "agent_prompt") return "Agent prompt";
  if (kind === "forum_post") return "Forum post";
  return "Shell command";
}

function blankDraft(workspaceID = "", conversationKey = "", forumChannelID = ""): TaskDraft {
  return {
    name: "",
    kind: "agent_prompt",
    enabled: true,
    schedule: "0 9 * * *",
    timezone: "UTC",
    conversationKey,
    forumChannelID,
    postTitle: "",
    freshContext: false,
    notify: true,
    agentID: "",
    workspaceID,
    prompt: "",
    command: "",
    timeoutSeconds: "600",
  };
}

function draftFromTask(
  task: ScheduledTask,
  workspaceID = "",
  fallbackConversationKey = "",
  fallbackForumChannelID = ""
): TaskDraft {
  const conversationKey =
    task.kind !== "forum_post" && task.conversation_type && task.conversation_id
      ? conversationKeyFor(task.conversation_type, task.conversation_id)
      : fallbackConversationKey;
  return {
    name: task.name,
    kind: task.kind,
    enabled: task.enabled,
    schedule: task.schedule,
    timezone: task.timezone || "UTC",
    conversationKey,
    forumChannelID: task.kind === "forum_post" ? task.conversation_id ?? "" : fallbackForumChannelID,
    postTitle: task.post_title ?? "",
    freshContext: task.fresh_context,
    notify: task.notify,
    agentID: task.agent_id ?? "",
    workspaceID: task.workspace_id ?? workspaceID,
    prompt: task.prompt ?? "",
    command: task.command ?? "",
    timeoutSeconds: String(task.timeout_seconds || 600),
  };
}

function payloadFromDraft(draft: TaskDraft): ScheduledTaskPayload {
  const conversation = parseConversationKey(draft.conversationKey);
  const isForumPost = draft.kind === "forum_post";
  const isShell = draft.kind === "shell_command";
  return {
    name: draft.name.trim(),
    kind: draft.kind,
    enabled: draft.enabled,
    schedule: draft.schedule.trim(),
    timezone: draft.timezone.trim() || "UTC",
    conversation_type: isShell ? "" : isForumPost ? "channel" : conversation?.type ?? "",
    conversation_id: isShell ? "" : isForumPost ? draft.forumChannelID : conversation?.id ?? "",
    agent_id: isShell ? "" : draft.agentID,
    workspace_id: draft.workspaceID.trim(),
    prompt: isShell ? "" : draft.prompt,
    command: isShell ? draft.command : "",
    post_title: isForumPost ? draft.postTitle.trim() : "",
    fresh_context: draft.kind === "agent_prompt" ? draft.freshContext : false,
    notify: isShell ? true : draft.notify,
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

async function resolveThread(
  queryClient: QueryClient,
  threadID: string,
  channelID: string | undefined,
  known: Thread[]
): Promise<Thread | undefined> {
  const local = known.find((thread) => thread.id === threadID);
  if (local || !channelID) return local;
  const list = await queryClient.fetchQuery({
    queryKey: ["threads", channelID],
    queryFn: () => channelThreads(channelID),
  });
  return list.find((thread) => thread.id === threadID);
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatDuration(startedAt: string, finishedAt?: string): string | undefined {
  if (!finishedAt) return undefined;
  const start = new Date(startedAt).getTime();
  const end = new Date(finishedAt).getTime();
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return undefined;
  const seconds = Math.round((end - start) / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

async function invalidateTasks(queryClient: QueryClient, projectID: string, taskID: string) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["scheduled-tasks", projectID] }),
    queryClient.invalidateQueries({ queryKey: ["scheduled-task-runs", taskID] }),
  ]);
}
