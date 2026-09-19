import { useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import type { Project } from "../../api/types";
import { getProjectAvatar, initials } from "./utils";

type ProjectSearchDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projects: Project[];
  currentProjectID?: string;
  onSelectProject: (projectID: string) => void;
};

export function filterProjects(projects: Project[], query: string): Project[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return projects;
  const prefix: Project[] = [];
  const contains: Project[] = [];
  for (const item of projects) {
    const name = item.name.toLowerCase();
    if (name.startsWith(needle)) prefix.push(item);
    else if (name.includes(needle)) contains.push(item);
  }
  return [...prefix, ...contains];
}

export function ProjectSearchDialog({
  open,
  onOpenChange,
  projects,
  currentProjectID,
  onSelectProject,
}: ProjectSearchDialogProps) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const results = useMemo(() => filterProjects(projects, query), [projects, query]);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActiveIndex(0);
    }
  }, [open]);

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  useEffect(() => {
    listRef.current
      ?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

  const choose = (projectID: string) => {
    onSelectProject(projectID);
    onOpenChange(false);
  };

  const handleKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.nativeEvent.isComposing) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (results.length) setActiveIndex((index) => (index + 1) % results.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      if (results.length) setActiveIndex((index) => (index - 1 + results.length) % results.length);
    } else if (event.key === "Enter") {
      event.preventDefault();
      const target = results[activeIndex];
      if (target) choose(target.id);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="top-[20%] translate-y-0 gap-0 overflow-hidden p-0 sm:max-w-md"
      >
        <DialogTitle className="sr-only">Search projects</DialogTitle>
        <DialogDescription className="sr-only">
          Type to filter projects, use arrow keys to move and Enter to open.
        </DialogDescription>
        <div className="flex items-center gap-2 border-b-2 border-border px-3">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <input
            autoFocus
            className="h-11 w-full min-w-0 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            placeholder="Search projects..."
            aria-label="Search projects"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={handleKeyDown}
          />
        </div>
        <div ref={listRef} role="listbox" aria-label="Projects" className="max-h-80 overflow-y-auto p-1.5">
          {results.length === 0 ? (
            <div className="px-3 py-6 text-center text-sm text-muted-foreground">No matching projects</div>
          ) : (
            results.map((item, index) => {
              const avatar = getProjectAvatar(item.id);
              const isActive = index === activeIndex;
              return (
                <button
                  key={item.id}
                  type="button"
                  role="option"
                  aria-selected={isActive}
                  data-index={index}
                  className={cn(
                    "flex w-full items-center gap-3 rounded-lg px-2 py-1.5 text-left text-sm",
                    isActive ? "bg-primary text-primary-foreground" : "text-foreground"
                  )}
                  onMouseMove={() => setActiveIndex(index)}
                  onClick={() => choose(item.id)}
                >
                  <span
                    className={cn(
                      "flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border-2 border-border text-xs font-semibold",
                      avatar?.emoji ? cn("text-white", avatar.color || "bg-primary") : "bg-card text-foreground"
                    )}
                  >
                    {avatar?.emoji ? <span className="text-sm">{avatar.emoji}</span> : initials(item.name)}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{item.name}</span>
                  {item.id === currentProjectID && (
                    <span className={cn("text-xs", isActive ? "text-primary-foreground/80" : "text-muted-foreground")}>
                      Current
                    </span>
                  )}
                </button>
              );
            })
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
