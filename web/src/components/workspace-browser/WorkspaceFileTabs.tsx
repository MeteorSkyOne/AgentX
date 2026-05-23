import { useState } from "react";
import type { DragEvent } from "react";
import { FileText, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import type { WorkspaceFileBrowserController } from "./types";

export function WorkspaceFileTabBar({
  controller,
}: {
  controller: WorkspaceFileBrowserController;
}) {
  const [dragTabId, setDragTabId] = useState<string | null>(null);
  const [dropIndicator, setDropIndicator] = useState<{ index: number; side: "left" | "right" } | null>(null);

  if (controller.tabs.length === 0) return null;

  function handleDragStart(e: DragEvent, tabId: string) {
    setDragTabId(tabId);
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("application/x-agentx-tab", tabId);
  }

  function handleDragOver(e: DragEvent, index: number) {
    if (!dragTabId) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    const rect = e.currentTarget.getBoundingClientRect();
    const midX = rect.left + rect.width / 2;
    const side = e.clientX < midX ? "left" : "right";
    setDropIndicator({ index, side });
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault();
    if (!dragTabId || !dropIndicator) return;
    const fromIndex = controller.tabs.findIndex((t) => t.id === dragTabId);
    if (fromIndex === -1) return;
    let toIndex = dropIndicator.side === "right" ? dropIndicator.index + 1 : dropIndicator.index;
    if (fromIndex < toIndex) toIndex -= 1;
    controller.reorderTabs(fromIndex, toIndex);
    setDragTabId(null);
    setDropIndicator(null);
  }

  function handleDragEnd() {
    setDragTabId(null);
    setDropIndicator(null);
  }

  return (
    <div
      className="flex shrink-0 items-center overflow-x-auto border-b border-border bg-muted/30"
      role="tablist"
      aria-label="Open files"
      style={{ scrollbarWidth: "none" }}
      onDrop={handleDrop}
      onDragOver={(e) => { if (dragTabId) e.preventDefault(); }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setDropIndicator(null);
        }
      }}
    >
      {controller.tabs.map((tab, index) => {
        const active = tab.id === controller.activeTabId;
        const dragging = tab.id === dragTabId;
        const fileName = tab.filePath.split("/").pop() || tab.filePath;
        const showLeftIndicator = dropIndicator?.index === index && dropIndicator.side === "left";
        const showRightIndicator = dropIndicator?.index === index && dropIndicator.side === "right";

        return (
          <ContextMenu key={tab.id}>
            <ContextMenuTrigger asChild>
              <button
                type="button"
                role="tab"
                aria-selected={active}
                title={tab.filePath}
                draggable
                onDragStart={(e) => handleDragStart(e, tab.id)}
                onDragEnd={handleDragEnd}
                onDragOver={(e) => handleDragOver(e, index)}
                className={cn(
                  "group relative flex shrink-0 items-center gap-1.5 border-r border-border px-3 py-1.5 text-xs outline-none transition-colors",
                  active
                    ? "bg-background text-foreground"
                    : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
                  dragging && "opacity-40"
                )}
                onClick={() => controller.switchTab(tab.id)}
                onDoubleClick={() => {
                  if (tab.preview) controller.pinTab(tab.id);
                }}
                onMouseDown={(e) => {
                  if (e.button === 1) {
                    e.preventDefault();
                    controller.closeTab(tab.id);
                  }
                }}
              >
                {showLeftIndicator && (
                  <span className="pointer-events-none absolute -left-px top-1 bottom-1 w-0.5 rounded-full bg-primary" />
                )}
                <FileText className="h-3 w-3 shrink-0 text-blue-400" />
                <span className={cn("max-w-40 truncate", tab.preview && "italic")}>
                  {fileName}
                </span>
                {tab.dirty ? (
                  <span
                    className={cn(
                      "ml-0.5 inline-block h-2 w-2 shrink-0 rounded-full bg-current",
                      "group-hover:hidden"
                    )}
                    aria-label="Unsaved changes"
                  />
                ) : null}
                <button
                  type="button"
                  className={cn(
                    "ml-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-sm hover:bg-accent",
                    !tab.dirty && "opacity-0 group-hover:opacity-100",
                    tab.dirty && "hidden group-hover:inline-flex"
                  )}
                  title="Close"
                  aria-label={`Close ${fileName}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    controller.closeTab(tab.id);
                  }}
                  draggable={false}
                >
                  <X className="h-3 w-3" />
                </button>
                {showRightIndicator && (
                  <span className="pointer-events-none absolute -right-px top-1 bottom-1 w-0.5 rounded-full bg-primary" />
                )}
              </button>
            </ContextMenuTrigger>
            <ContextMenuContent>
              <ContextMenuItem onSelect={() => controller.closeTab(tab.id)}>
                Close
              </ContextMenuItem>
              <ContextMenuItem onSelect={() => controller.closeOtherTabs(tab.id)}>
                Close Others
              </ContextMenuItem>
              <ContextMenuItem onSelect={() => controller.closeAllTabs()}>
                Close All
              </ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuItem
                onSelect={() => {
                  void navigator.clipboard.writeText(tab.filePath);
                }}
              >
                Copy Path
              </ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        );
      })}
    </div>
  );
}
