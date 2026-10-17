import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Circle,
  CircleCheck,
  CircleDot,
  ListChecks,
  LoaderCircle,
  Target,
} from "lucide-react";
import type { AgentGoal, AgentPlan, AgentPlanStep } from "@/api/types";
import { cn } from "@/lib/utils";
import { formatDuration, formatTokenCount } from "../messageMetrics";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";

/**
 * The agent's current goal and step list, shown under a streaming reply (live)
 * and under the finished reply (final state).
 */
export function RunProgress({
  goal,
  plan,
  live = false,
}: {
  goal?: AgentGoal;
  plan?: AgentPlan;
  live?: boolean;
}) {
  const hasPlan = Boolean(plan && plan.steps.length > 0);
  if (!goal && !hasPlan) {
    return null;
  }
  return (
    <div className="space-y-2 rounded-md border border-border/60 bg-muted/30 px-3 py-2 text-xs">
      {goal && <GoalSummary goal={goal} live={live} />}
      {plan && hasPlan && <PlanSteps plan={plan} live={live} />}
    </div>
  );
}

function GoalSummary({ goal, live }: { goal: AgentGoal; live: boolean }) {
  const { label, tone } = goalStatusLabel(goal);
  const usage = goalUsageParts(goal);
  return (
    <div className="space-y-1" data-testid="run-goal">
      <div className="flex flex-wrap items-center gap-1.5">
        <Target className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span
          className={cn(
            "font-medium",
            tone === "active" && "text-blue-500 dark:text-blue-400",
            tone === "success" && "text-emerald-600 dark:text-emerald-400",
            tone === "warning" && "text-amber-600 dark:text-amber-400",
            tone === "muted" && "text-muted-foreground",
            tone === "active" && live && "animate-pulse"
          )}
        >
          {label}
        </span>
        {usage.length > 0 && (
          <span className="text-muted-foreground">{usage.join(" · ")}</span>
        )}
      </div>
      <p className="whitespace-pre-wrap break-words text-foreground/90">{goal.objective}</p>
      {goal.last_reason && (
        <p className="whitespace-pre-wrap break-words text-muted-foreground">
          <span className="font-medium">Last check: </span>
          {goal.last_reason}
        </p>
      )}
    </div>
  );
}

function PlanSteps({ plan, live }: { plan: AgentPlan; live: boolean }) {
  const [open, setOpen] = useState(live);
  const completed = plan.steps.filter((step) => step.status === "completed").length;
  const current = plan.steps.find((step) => step.status === "in_progress");
  return (
    <Collapsible open={open} onOpenChange={setOpen} data-testid="run-plan">
      <CollapsibleTrigger className="flex w-full min-w-0 items-center gap-1.5 text-left text-muted-foreground hover:text-foreground">
        {open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
        <ListChecks className="h-3.5 w-3.5 shrink-0" />
        <span className="font-medium">Steps</span>
        <span>
          {completed}/{plan.steps.length}
        </span>
        {!open && current && <span className="min-w-0 truncate text-foreground/80">· {current.step}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent>
        {plan.explanation && (
          <p className="mt-1 whitespace-pre-wrap break-words italic text-muted-foreground">{plan.explanation}</p>
        )}
        <ol className="mt-1 space-y-0.5">
          {plan.steps.map((step, index) => (
            <PlanStepRow key={`${index}:${step.step}`} step={step} live={live} />
          ))}
        </ol>
      </CollapsibleContent>
    </Collapsible>
  );
}

function PlanStepRow({ step, live }: { step: AgentPlanStep; live: boolean }) {
  const status = step.status;
  return (
    <li className="flex items-start gap-1.5" data-status={status}>
      {status === "completed" ? (
        <CircleCheck className="mt-px h-3.5 w-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" />
      ) : status === "in_progress" ? (
        live ? (
          <LoaderCircle className="mt-px h-3.5 w-3.5 shrink-0 animate-spin text-blue-500 dark:text-blue-400" />
        ) : (
          <CircleDot className="mt-px h-3.5 w-3.5 shrink-0 text-blue-500 dark:text-blue-400" />
        )
      ) : (
        <Circle className="mt-px h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
      )}
      <span
        className={cn(
          "min-w-0 break-words",
          status === "completed" && "text-muted-foreground line-through",
          status === "in_progress" && "font-medium text-foreground",
          status !== "completed" && status !== "in_progress" && "text-foreground/80"
        )}
      >
        {step.step}
      </span>
    </li>
  );
}

type GoalTone = "active" | "success" | "warning" | "muted";

export function goalStatusLabel(goal: AgentGoal): { label: string; tone: GoalTone } {
  switch (goal.status) {
    case "active":
      return { label: "Pursuing goal", tone: "active" };
    case "complete":
      return { label: "Goal achieved", tone: "success" };
    case "paused":
      return { label: "Goal paused", tone: "warning" };
    case "blocked":
      return { label: "Goal stalled", tone: "warning" };
    case "usage_limited":
      return { label: "Goal hit usage limits", tone: "warning" };
    case "budget_limited":
      return { label: isFiniteNumber(goal.token_budget) ? "Goal unmet" : "Goal abandoned", tone: "warning" };
    case "cleared":
      return { label: "Goal cleared", tone: "muted" };
    default:
      return { label: `Goal ${goal.status}`, tone: "muted" };
  }
}

/** Usage summary, e.g. ["12k / 50k tokens", "2m 5s", "2 unmet checks"]. */
export function goalUsageParts(goal: AgentGoal): string[] {
  const parts: string[] = [];
  if (isFiniteNumber(goal.tokens_used) && goal.tokens_used > 0) {
    parts.push(
      isFiniteNumber(goal.token_budget)
        ? `${formatTokenCount(goal.tokens_used)} / ${formatTokenCount(goal.token_budget)} tokens`
        : `${formatTokenCount(goal.tokens_used)} tokens`
    );
  } else if (isFiniteNumber(goal.token_budget)) {
    parts.push(`${formatTokenCount(goal.token_budget)} token budget`);
  }
  if (isFiniteNumber(goal.time_used_seconds) && goal.time_used_seconds > 0) {
    parts.push(formatDuration(goal.time_used_seconds * 1000));
  }
  if (isFiniteNumber(goal.iterations) && goal.iterations > 0) {
    parts.push(`${goal.iterations} unmet ${goal.iterations === 1 ? "check" : "checks"}`);
  }
  return parts;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
