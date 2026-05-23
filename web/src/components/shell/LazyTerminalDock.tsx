import { Suspense, lazy, type ComponentProps } from "react";

const LazyTerminalDock = lazy(() => import("../TerminalDock").then((module) => ({ default: module.TerminalDock })));

export function TerminalFallback() {
  return (
    <section className="flex h-full min-h-0 flex-1 flex-col bg-background" aria-label="Terminal">
      <div className="h-11 shrink-0 border-b border-border bg-sidebar" />
      <div className="flex min-h-0 flex-1 items-center justify-center bg-[#111315] text-sm text-muted-foreground">
        Loading terminal...
      </div>
    </section>
  );
}

export function TerminalDockBoundary(props: ComponentProps<typeof LazyTerminalDock>) {
  return (
    <Suspense fallback={<TerminalFallback />}>
      <LazyTerminalDock {...props} />
    </Suspense>
  );
}
