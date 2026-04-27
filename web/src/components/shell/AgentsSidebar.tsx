import { useState } from "react";
import { ChevronDown, ChevronRight, Plus, Settings } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import type { Agent, ConversationAgentContext } from "../../api/types";
import { AgentAvatar } from "../AgentAvatar";

export function AgentsSidebar({
  agents,
  boundAgents,
  contextLoading,
  onOpenPanel,
  onCreateAgent,
}: {
  agents: Agent[];
  boundAgents: ConversationAgentContext[];
  contextLoading: boolean;
  onOpenPanel: (agentID?: string) => void;
  onCreateAgent: () => void;
}) {
  const [open, setOpen] = useState(true);

  return (
    <section aria-label="Bound agent" className="min-w-0 max-w-full overflow-hidden">
      <Collapsible open={open} onOpenChange={setOpen} className="mt-4 min-w-0 max-w-full overflow-hidden">
        <CollapsibleTrigger asChild>
          <button className="flex min-w-0 max-w-full w-full items-center gap-1 px-1 py-1 text-xs font-semibold uppercase text-muted-foreground hover:text-foreground">
            {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            Agents
            {contextLoading && <span className="ml-1 h-1.5 w-1.5 rounded-full bg-yellow-500 animate-pulse" />}
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="min-w-0 max-w-full space-y-0.5 overflow-hidden">
          {boundAgents.map((item) => (
            <button
              key={item.agent.id}
              className="group flex min-h-10 min-w-0 max-w-full w-full items-center gap-2 overflow-hidden rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-foreground md:min-h-0"
              aria-label={item.agent.name}
              onClick={() => onOpenPanel(item.agent.id)}
            >
              <div className="relative">
                <AgentAvatar agentID={item.agent.id} kind={item.agent.kind} size="sm" className="h-5 w-5" />
                <div className={cn(
                  "absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border border-card",
                  item.agent.enabled ? "bg-green-500" : "bg-gray-500"
                )} />
              </div>
              <span className="min-w-0 max-w-[calc(100svw-8rem)] truncate">{item.agent.name}</span>
              <Settings className="ml-auto h-3 w-3 opacity-0 group-hover:opacity-100" />
            </button>
          ))}
          {boundAgents.length === 0 && (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">Unbound</p>
          )}
          <button
              className="flex min-h-10 min-w-0 max-w-full w-full items-center gap-2 overflow-hidden rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground md:min-h-0"
            onClick={onCreateAgent}
          >
            <Plus className="h-4 w-4 shrink-0" />
            <span className="min-w-0 truncate">Create agent</span>
          </button>
          <button
            className="flex min-h-10 min-w-0 max-w-full w-full items-center gap-2 overflow-hidden rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground md:min-h-0"
            onClick={() => onOpenPanel()}
          >
            <Settings className="h-4 w-4 shrink-0" />
            <span className="min-w-0 truncate">Manage agents</span>
          </button>
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}
