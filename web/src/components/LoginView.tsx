import { FormEvent, useState } from "react";
import { LogIn } from "lucide-react";
import { login, setToken } from "../api/client";
import type { AuthResponse } from "../api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface LoginViewProps {
  onAuthenticated: (result: AuthResponse) => void;
}

export function LoginView({ onAuthenticated }: LoginViewProps) {
  const [adminToken, setAdminToken] = useState("");
  const [displayName, setDisplayName] = useState("Admin");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const result = await login(adminToken, displayName);
      setToken(result.session_token);
      onAuthenticated(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex min-h-dvh w-screen items-center justify-center bg-background p-4">
      <form
        className="flex w-full max-w-sm flex-col gap-6 rounded-xl border border-border bg-card p-6 sm:p-8"
        onSubmit={handleSubmit}
      >
        <div className="flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-lg">
            AX
          </div>
          <div>
            <h1 className="text-xl font-bold">AgentX</h1>
            <p className="text-sm text-muted-foreground">Foundation workspace</p>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="admin-token">Admin token</Label>
          <Input
            id="admin-token"
            autoFocus
            value={adminToken}
            onChange={(event) => setAdminToken(event.target.value)}
            type="password"
            autoComplete="current-password"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="display-name">Display name</Label>
          <Input
            id="display-name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            autoComplete="name"
          />
        </div>

        {error ? <p className="text-sm text-destructive">{error}</p> : null}

        <Button disabled={submitting || adminToken.trim() === ""} className="w-full gap-2">
          <LogIn className="h-4 w-4" />
          <span>{submitting ? "Bootstrapping..." : "Enter"}</span>
        </Button>
      </form>
    </main>
  );
}
