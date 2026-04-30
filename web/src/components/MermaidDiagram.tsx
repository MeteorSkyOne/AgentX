import { useEffect, useId, useRef, useState } from "react";

interface MermaidState {
  status: "loading" | "rendered" | "error";
  svg?: string;
  error?: string;
}

let mermaidInitialized = false;
let mermaidRenderQueue: Promise<unknown> = Promise.resolve();

export function resetMermaidDiagramForTests() {
  mermaidInitialized = false;
  mermaidRenderQueue = Promise.resolve();
}

export function MermaidDiagram({ source }: { source: string }) {
  const reactID = useId();
  const renderID = `agentx-mermaid-${reactID.replace(/[^A-Za-z0-9_-]/g, "")}`;
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<MermaidState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    async function render() {
      try {
        const mermaid = (await import("mermaid")).default;
        if (!mermaidInitialized) {
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: "strict",
          });
          mermaidInitialized = true;
        }
        const result = await enqueueMermaidRender(() => mermaid.render(renderID, source));
        if (!result.svg.trim()) {
          throw new Error("Mermaid returned empty SVG output");
        }
        if (cancelled) return;
        setState({ status: "rendered", svg: result.svg });
        requestAnimationFrame(() => {
          if (cancelled || !containerRef.current || !result.bindFunctions) return;
          result.bindFunctions(containerRef.current);
        });
      } catch (err) {
        if (cancelled) return;
        setState({
          status: "error",
          error: err instanceof Error ? err.message : "Mermaid render failed",
        });
      }
    }

    void render();
    return () => {
      cancelled = true;
    };
  }, [renderID, source]);

  if (state.status === "error") {
    return (
      <DiagramError
        title="Mermaid render failed"
        message={state.error ?? "Mermaid render failed"}
        source={source}
      />
    );
  }

  return (
    <div
      className="my-3 min-w-0 max-w-full overflow-x-auto rounded-md border border-border bg-background p-3"
      data-testid="mermaid-diagram"
      aria-label="Mermaid diagram"
    >
      {state.status === "loading" ? (
        <div className="text-xs text-muted-foreground" role="status">
          Rendering diagram...
        </div>
      ) : (
        <div
          ref={containerRef}
          className="min-w-fit [&_svg]:max-w-full [&_svg]:h-auto"
          dangerouslySetInnerHTML={{ __html: state.svg ?? "" }}
        />
      )}
    </div>
  );
}

function enqueueMermaidRender<T>(render: () => Promise<T>): Promise<T> {
  const next = mermaidRenderQueue.then(render, render);
  mermaidRenderQueue = next.catch(() => undefined);
  return next;
}

function DiagramError({
  title,
  message,
  source,
}: {
  title: string;
  message: string;
  source: string;
}) {
  return (
    <div
      className="my-3 rounded-md border border-destructive/30 bg-destructive/5 p-3"
      data-testid="diagram-error"
      role="alert"
    >
      <div className="text-sm font-medium text-destructive">{title}</div>
      <div className="mt-1 text-xs text-muted-foreground">{message}</div>
      <pre className="mt-3 max-h-64 overflow-auto rounded bg-background p-2 text-xs">
        <code>{source}</code>
      </pre>
    </div>
  );
}
