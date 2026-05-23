export function LoadingSessionView({ onClearSession }: { onClearSession: () => void }) {
  return (
    <main className="flex h-screen w-screen items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-4">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-lg">
          AX
        </div>
        <span className="text-sm text-muted-foreground">Loading session...</span>
        <button
          className="text-sm text-muted-foreground hover:text-foreground underline"
          type="button"
          onClick={onClearSession}
        >
          Clear session
        </button>
      </div>
    </main>
  );
}
