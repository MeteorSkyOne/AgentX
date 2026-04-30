import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type WheelEvent,
} from "react";
import { RotateCcw, ZoomIn, ZoomOut } from "lucide-react";
import { renderD2Diagram } from "@/api/client";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const d2CacheTTL = 24 * 60 * 60 * 1000;
const d2CacheMaxEntries = 128;
const diagramMaxHeight = "min(60svh, 32rem)";
const previewMinScale = 0.25;
const previewMaxScale = 50;

interface D2CacheEntry {
  svg: string;
  expiresAt: number;
}

interface D2State {
  status: "loading" | "rendered" | "error";
  url?: string;
  error?: string;
}

const svgCache = new Map<string, D2CacheEntry>();
const inFlight = new Map<string, Promise<string>>();

interface PreviewPan {
  x: number;
  y: number;
}

export function D2Diagram({ source }: { source: string }) {
  const [state, setState] = useState<D2State>({ status: "loading" });
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewScale, setPreviewScale] = useState(1);
  const [previewSize, setPreviewSize] = useState<{ width: number; height: number } | null>(null);
  const [previewPan, setPreviewPan] = useState<PreviewPan>({ x: 0, y: 0 });
  const previewViewportRef = useRef<HTMLDivElement | null>(null);
  const previewDragRef = useRef<{
    pointerId: number;
    startX: number;
    startY: number;
    panX: number;
    panY: number;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    let objectURL: string | undefined;
    setState({ status: "loading" });

    async function render() {
      try {
        const svg = await cachedD2SVG(source);
        if (cancelled) return;
        objectURL = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }));
        setState({ status: "rendered", url: objectURL });
      } catch (err) {
        if (cancelled) return;
        setState({
          status: "error",
          error: err instanceof Error ? err.message : "D2 render failed",
        });
      }
    }

    void render();
    return () => {
      cancelled = true;
      if (objectURL) {
        URL.revokeObjectURL(objectURL);
      }
    };
  }, [source]);

  useEffect(() => {
    setPreviewOpen(false);
    setPreviewScale(1);
    setPreviewSize(null);
    setPreviewPan({ x: 0, y: 0 });
  }, [source]);

  useEffect(() => {
    const viewport = previewViewportRef.current;
    if (!viewport || previewSize === null) {
      return;
    }

    setPreviewPan((pan) => clampD2PreviewPan(pan, previewScale, previewSize, viewport));
  }, [previewScale, previewSize]);

  if (state.status === "error") {
    return (
      <DiagramError
        title="D2 render failed"
        message={state.error ?? "D2 render failed"}
        source={source}
      />
    );
  }

  const imageURL = state.url;

  function openPreview() {
    if (imageURL) {
      setPreviewPan({ x: 0, y: 0 });
      setPreviewOpen(true);
    }
  }

  function openPreviewWithKeyboard(event: KeyboardEvent<HTMLImageElement>) {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openPreview();
  }

  function zoomPreview(nextScale: number) {
    setPreviewScale(clampD2PreviewScale(nextScale));
  }

  function zoomPreviewWithWheel(event: WheelEvent<HTMLDivElement>) {
    event.preventDefault();
    setPreviewScale((scale) => nextD2PreviewScale(scale, event.deltaY));
  }

  function handlePreviewPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (previewSize === null || (event.pointerType === "mouse" && event.button !== 0)) {
      return;
    }
    event.preventDefault();
    if (event.currentTarget.setPointerCapture) {
      event.currentTarget.setPointerCapture(event.pointerId);
    }
    previewDragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      panX: previewPan.x,
      panY: previewPan.y,
    };
  }

  function handlePreviewPointerMove(event: PointerEvent<HTMLDivElement>) {
    const drag = previewDragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    const viewport = previewViewportRef.current;
    const nextPan = {
      x: drag.panX + event.clientX - drag.startX,
      y: drag.panY + event.clientY - drag.startY,
    };
    setPreviewPan(clampD2PreviewPan(nextPan, previewScale, previewSize, viewport));
  }

  function stopPreviewDrag(event: PointerEvent<HTMLDivElement>) {
    if (previewDragRef.current?.pointerId === event.pointerId) {
      previewDragRef.current = null;
      if (event.currentTarget.releasePointerCapture) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    }
  }

  const previewScaledSize =
    previewSize === null
      ? null
      : {
          width: Math.round(previewSize.width * previewScale),
          height: Math.round(previewSize.height * previewScale),
        };
  const previewImageStyle =
    previewScaledSize === null
      ? undefined
      : {
          width: `${previewScaledSize.width}px`,
          height: `${previewScaledSize.height}px`,
          transform: `translate(-50%, -50%) translate(${previewPan.x}px, ${previewPan.y}px)`,
        };

  return (
    <>
      <div
        className="my-3 min-w-0 max-w-full overflow-hidden rounded-md border border-border bg-background"
        data-testid="d2-diagram"
        aria-label="D2 diagram"
      >
        {state.status === "loading" ? (
          <div className="p-3 text-xs text-muted-foreground" role="status">
            Rendering diagram...
          </div>
        ) : (
          <>
            <div className="flex items-center justify-end gap-1 border-b border-border bg-muted/30 px-2 py-1">
              <button
                type="button"
                className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label="Open D2 diagram preview"
                title="Open D2 diagram preview"
                onClick={openPreview}
              >
                <ZoomIn className="h-4 w-4" />
              </button>
            </div>
            <div
              className="min-w-0 max-w-full overflow-auto p-3"
              data-testid="d2-diagram-viewport"
              style={{ maxHeight: diagramMaxHeight }}
            >
              <img
                src={imageURL}
                alt="D2 diagram"
                className="block h-auto max-w-none cursor-zoom-in"
                data-testid="d2-diagram-image"
                role="button"
                tabIndex={0}
                aria-label="Open D2 diagram preview"
                onClick={openPreview}
                onKeyDown={openPreviewWithKeyboard}
              />
            </div>
          </>
        )}
      </div>

      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="flex h-[min(92vh,64rem)] max-w-[calc(100vw-1rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-7xl">
          <DialogHeader className="shrink-0 border-b border-border px-4 py-3 pr-12">
            <div className="flex min-w-0 items-center gap-3">
              <div className="min-w-0 flex-1">
                <DialogTitle className="truncate text-sm">D2 Diagram</DialogTitle>
                <DialogDescription>{Math.round(previewScale * 100)}%</DialogDescription>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label="Zoom out D2 diagram"
                  title="Zoom out D2 diagram"
                  onClick={() => zoomPreview(previewScale / 1.25)}
                >
                  <ZoomOut className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label="Reset D2 diagram zoom"
                  title="Reset D2 diagram zoom"
                  onClick={() => {
                    zoomPreview(1);
                    setPreviewPan({ x: 0, y: 0 });
                  }}
                >
                  <RotateCcw className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label="Zoom in D2 diagram"
                  title="Zoom in D2 diagram"
                  onClick={() => zoomPreview(previewScale * 1.25)}
                >
                  <ZoomIn className="h-4 w-4" />
                </button>
              </div>
            </div>
          </DialogHeader>
          <div
            ref={previewViewportRef}
            className="relative min-h-0 flex-1 cursor-grab overflow-hidden bg-muted/20 touch-none select-none active:cursor-grabbing"
            data-testid="d2-diagram-preview-viewport"
            onWheel={zoomPreviewWithWheel}
            onPointerDown={handlePreviewPointerDown}
            onPointerMove={handlePreviewPointerMove}
            onPointerUp={stopPreviewDrag}
            onPointerCancel={stopPreviewDrag}
          >
            {imageURL ? (
              <div
                className="relative h-full w-full"
                data-testid="d2-diagram-preview-canvas"
              >
                <img
                  src={imageURL}
                  alt="D2 diagram preview"
                  className="absolute left-1/2 top-1/2 block h-auto max-w-none"
                  data-testid="d2-diagram-preview-image"
                  draggable={false}
                  style={previewImageStyle}
                  onLoad={(event) => {
                    const img = event.currentTarget;
                    if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                      setPreviewSize({
                        width: img.naturalWidth,
                        height: img.naturalHeight,
                      });
                    }
                  }}
                />
              </div>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

export function clampD2PreviewScale(scale: number): number {
  if (!Number.isFinite(scale)) {
    return 1;
  }
  return Math.min(previewMaxScale, Math.max(previewMinScale, Number(scale.toFixed(3))));
}

export function nextD2PreviewScale(currentScale: number, deltaY: number): number {
  const multiplier = Math.exp(-deltaY * 0.0015);
  return clampD2PreviewScale(currentScale * multiplier);
}

function clampD2PreviewPan(
  pan: PreviewPan,
  scale: number,
  size: { width: number; height: number } | null,
  viewport: HTMLDivElement | null
): PreviewPan {
  if (size === null || viewport === null) {
    return pan;
  }
  const maxX = Math.max(0, (size.width * scale - viewport.clientWidth) / 2);
  const maxY = Math.max(0, (size.height * scale - viewport.clientHeight) / 2);
  return {
    x: clampNumber(pan.x, -maxX, maxX),
    y: clampNumber(pan.y, -maxY, maxY),
  };
}

function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export async function cachedD2SVG(source: string): Promise<string> {
  const key = await d2SourceHash(source);
  const now = Date.now();
  const hit = svgCache.get(key);
  if (hit && now < hit.expiresAt) {
    svgCache.delete(key);
    svgCache.set(key, hit);
    return hit.svg;
  }
  if (hit) {
    svgCache.delete(key);
  }

  const pending = inFlight.get(key);
  if (pending) {
    return pending;
  }

  const promise = renderD2Diagram(source)
    .then((result) => {
      storeD2SVG(key, result.svg, Date.now());
      return result.svg;
    })
    .finally(() => {
      inFlight.delete(key);
    });
  inFlight.set(key, promise);
  return promise;
}

export function clearD2DiagramCacheForTests() {
  svgCache.clear();
  inFlight.clear();
}

async function d2SourceHash(source: string): Promise<string> {
  const subtle = globalThis.crypto?.subtle;
  if (subtle) {
    const data = new TextEncoder().encode(source);
    const digest = await subtle.digest("SHA-256", data);
    return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
  }

  let hash = 0x811c9dc5;
  for (let i = 0; i < source.length; i += 1) {
    hash ^= source.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return `fnv-${(hash >>> 0).toString(16)}`;
}

function storeD2SVG(key: string, svg: string, now: number) {
  svgCache.delete(key);
  svgCache.set(key, { svg, expiresAt: now + d2CacheTTL });
  for (const [cacheKey, entry] of svgCache) {
    if (now >= entry.expiresAt || svgCache.size > d2CacheMaxEntries) {
      svgCache.delete(cacheKey);
    } else {
      break;
    }
  }
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
