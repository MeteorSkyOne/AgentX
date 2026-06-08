import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Circle,
  Map,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import {
  createRoadmapStage,
  createRoadmapTask,
  deleteRoadmapStage,
  deleteRoadmapTask,
  projectRoadmap,
  updateRoadmapStage,
  updateRoadmapTask,
} from "../../api/client";
import type { Project, RoadmapStageWithTasks } from "../../api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";

export function RoadmapPanel({ project }: { project?: Project }) {
  const queryClient = useQueryClient();
  const roadmapQuery = useQuery({
    queryKey: ["roadmap", project?.id],
    queryFn: () => projectRoadmap(project!.id),
    enabled: Boolean(project?.id),
    refetchInterval: 10000,
  });

  const [newStageName, setNewStageName] = useState("");
  const [addingStage, setAddingStage] = useState(false);
  const [newTaskInputs, setNewTaskInputs] = useState<Record<string, string>>({});
  const [collapsedStages, setCollapsedStages] = useState<Set<string>>(new Set());
  const [editingStage, setEditingStage] = useState<string | null>(null);
  const [editStageName, setEditStageName] = useState("");
  const [editingTask, setEditingTask] = useState<string | null>(null);
  const [editTaskTitle, setEditTaskTitle] = useState("");

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["roadmap", project?.id] });

  const handleCreateStage = async () => {
    if (!project || !newStageName.trim()) return;
    await createRoadmapStage(project.id, { name: newStageName.trim() });
    setNewStageName("");
    setAddingStage(false);
    invalidate();
  };

  const handleDeleteStage = async (stageID: string) => {
    await deleteRoadmapStage(stageID);
    invalidate();
  };

  const handleToggleStageStatus = async (entry: RoadmapStageWithTasks) => {
    const newStatus = entry.stage.status === "active" ? "completed" : "active";
    await updateRoadmapStage(entry.stage.id, { status: newStatus });
    invalidate();
  };

  const handleStartEditStage = (stageID: string, currentName: string) => {
    setEditingStage(stageID);
    setEditStageName(currentName);
  };

  const handleSaveStageEdit = async () => {
    if (!editingStage || !editStageName.trim()) return;
    await updateRoadmapStage(editingStage, { name: editStageName.trim() });
    setEditingStage(null);
    invalidate();
  };

  const handleCreateTask = async (stageID: string) => {
    const title = (newTaskInputs[stageID] ?? "").trim();
    if (!title) return;
    await createRoadmapTask(stageID, { title });
    setNewTaskInputs((prev) => ({ ...prev, [stageID]: "" }));
    invalidate();
  };

  const handleToggleTask = async (taskID: string, completed: boolean) => {
    await updateRoadmapTask(taskID, { completed: !completed });
    invalidate();
  };

  const handleDeleteTask = async (taskID: string) => {
    await deleteRoadmapTask(taskID);
    invalidate();
  };

  const handleStartEditTask = (taskID: string, currentTitle: string) => {
    setEditingTask(taskID);
    setEditTaskTitle(currentTitle);
  };

  const handleSaveTaskEdit = async () => {
    if (!editingTask || !editTaskTitle.trim()) return;
    await updateRoadmapTask(editingTask, { title: editTaskTitle.trim() });
    setEditingTask(null);
    invalidate();
  };

  const toggleCollapse = (stageID: string) => {
    setCollapsedStages((prev) => {
      const next = new Set(prev);
      if (next.has(stageID)) next.delete(stageID);
      else next.add(stageID);
      return next;
    });
  };

  if (!project) {
    return (
      <section className="flex min-h-0 flex-1 flex-col bg-background">
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Select a project to view its roadmap.
        </div>
      </section>
    );
  }

  const stages = roadmapQuery.data ?? [];

  return (
    <section className="flex min-h-0 flex-1 flex-col bg-background">
      <div className="flex shrink-0 items-center justify-between border-b border-border px-4 py-2">
        <div className="flex items-center gap-2">
          <Map className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-medium">Roadmap</h2>
          {stages.length > 0 && (
            <span className="text-xs text-muted-foreground">
              {stages.length} {stages.length === 1 ? "stage" : "stages"}
            </span>
          )}
        </div>
        <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" onClick={() => setAddingStage(true)}>
          <Plus className="h-3.5 w-3.5" />
          Add stage
        </Button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="space-y-4 p-4">
          {stages.map((entry) => {
            const completedCount = entry.tasks.filter((t) => t.completed).length;
            const totalCount = entry.tasks.length;
            const isCollapsed = collapsedStages.has(entry.stage.id);

            return (
              <div key={entry.stage.id} className="rounded-lg border border-border">
                <div className="flex items-center gap-2 px-3 py-2">
                  <button
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    onClick={() => toggleCollapse(entry.stage.id)}
                  >
                    {isCollapsed ? (
                      <ChevronRight className="h-4 w-4" />
                    ) : (
                      <ChevronDown className="h-4 w-4" />
                    )}
                  </button>
                  {editingStage === entry.stage.id ? (
                    <Input
                      className="h-7 flex-1 text-sm"
                      value={editStageName}
                      onChange={(e) => setEditStageName(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") handleSaveStageEdit();
                        if (e.key === "Escape") setEditingStage(null);
                      }}
                      onBlur={handleSaveStageEdit}
                      autoFocus
                    />
                  ) : (
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">{entry.stage.name}</span>
                  )}
                  <Badge
                    variant={entry.stage.status === "completed" ? "default" : "secondary"}
                    className="cursor-pointer select-none text-[10px]"
                    onClick={() => handleToggleStageStatus(entry)}
                  >
                    {entry.stage.status === "completed" ? (
                      <CheckCircle2 className="mr-1 h-3 w-3" />
                    ) : (
                      <Circle className="mr-1 h-3 w-3" />
                    )}
                    {entry.stage.status}
                  </Badge>
                  {totalCount > 0 && (
                    <span className="text-[11px] text-muted-foreground">
                      {completedCount}/{totalCount}
                    </span>
                  )}
                  {totalCount > 0 && (
                    <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full bg-primary transition-all"
                        style={{ width: `${(completedCount / totalCount) * 100}%` }}
                      />
                    </div>
                  )}
                  <div className="flex shrink-0 items-center gap-0.5">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-6 w-6 opacity-0 group-hover/stage:opacity-100 hover:opacity-100 focus:opacity-100"
                      onClick={() => handleStartEditStage(entry.stage.id, entry.stage.name)}
                    >
                      <Pencil className="h-3 w-3" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-6 w-6 text-destructive opacity-0 group-hover/stage:opacity-100 hover:opacity-100 focus:opacity-100"
                      onClick={() => handleDeleteStage(entry.stage.id)}
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                </div>

                {!isCollapsed && (
                  <div className="border-t border-border px-3 py-1">
                    {entry.tasks.map((task) => (
                      <div
                        key={task.id}
                        className="group/task flex items-center gap-2 rounded px-1 py-1 hover:bg-accent/50"
                      >
                        <Checkbox
                          checked={task.completed}
                          onChange={() => handleToggleTask(task.id, task.completed)}
                          className="shrink-0"
                        />
                        {editingTask === task.id ? (
                          <Input
                            className="h-6 flex-1 text-sm"
                            value={editTaskTitle}
                            onChange={(e) => setEditTaskTitle(e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") handleSaveTaskEdit();
                              if (e.key === "Escape") setEditingTask(null);
                            }}
                            onBlur={handleSaveTaskEdit}
                            autoFocus
                          />
                        ) : (
                          <span
                            className={cn(
                              "min-w-0 flex-1 truncate text-sm",
                              task.completed && "text-muted-foreground line-through"
                            )}
                          >
                            {task.title}
                          </span>
                        )}
                        <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover/task:opacity-100">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-5 w-5"
                            onClick={() => handleStartEditTask(task.id, task.title)}
                          >
                            <Pencil className="h-2.5 w-2.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-5 w-5 text-destructive"
                            onClick={() => handleDeleteTask(task.id)}
                          >
                            <Trash2 className="h-2.5 w-2.5" />
                          </Button>
                        </div>
                      </div>
                    ))}
                    <div className="flex items-center gap-2 py-1">
                      <Plus className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <Input
                        className="h-6 flex-1 border-none bg-transparent text-sm shadow-none placeholder:text-muted-foreground/60 focus-visible:ring-0"
                        placeholder="Add a task..."
                        value={newTaskInputs[entry.stage.id] ?? ""}
                        onChange={(e) =>
                          setNewTaskInputs((prev) => ({
                            ...prev,
                            [entry.stage.id]: e.target.value,
                          }))
                        }
                        onKeyDown={(e) => {
                          if (e.key === "Enter") handleCreateTask(entry.stage.id);
                        }}
                      />
                    </div>
                  </div>
                )}
              </div>
            );
          })}

          {addingStage && (
            <div className="rounded-lg border border-dashed border-border p-3">
              <Input
                className="h-8 text-sm"
                placeholder="Stage name..."
                value={newStageName}
                onChange={(e) => setNewStageName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleCreateStage();
                  if (e.key === "Escape") {
                    setAddingStage(false);
                    setNewStageName("");
                  }
                }}
                autoFocus
              />
              <div className="mt-2 flex gap-2">
                <Button size="sm" className="h-7 text-xs" onClick={handleCreateStage}>
                  Create
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 text-xs"
                  onClick={() => {
                    setAddingStage(false);
                    setNewStageName("");
                  }}
                >
                  Cancel
                </Button>
              </div>
            </div>
          )}

          {stages.length === 0 && !addingStage && (
            <div className="flex flex-col items-center gap-3 py-12 text-center">
              <Map className="h-8 w-8 text-muted-foreground/50" />
              <div>
                <p className="text-sm font-medium">No roadmap yet</p>
                <p className="text-xs text-muted-foreground">
                  Add stages to organize your project goals and track progress.
                </p>
              </div>
              <Button variant="outline" size="sm" className="h-7 gap-1 text-xs" onClick={() => setAddingStage(true)}>
                <Plus className="h-3.5 w-3.5" />
                Add first stage
              </Button>
            </div>
          )}
        </div>
      </ScrollArea>
    </section>
  );
}
