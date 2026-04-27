import { FormEvent, useEffect, useState } from "react";
import { LogIn, UserPlus } from "lucide-react";
import { authStatus, login, setToken, setupAdmin } from "../api/client";
import type { AuthResponse, AuthStatus } from "../api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface LoginViewProps {
  onAuthenticated: (result: AuthResponse) => void;
}

export function LoginView({ onAuthenticated }: LoginViewProps) {
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [setupToken, setSetupToken] = useState("");
  const [username, setUsername] = useState("admin");
  const [displayName, setDisplayName] = useState("Admin");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    authStatus()
      .then((result) => {
        if (!cancelled) {
          setStatus(result);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Unable to load auth status");
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!status) {
      return;
    }
    setError(null);
    const setupRequired = status.setup_required;
    if (setupRequired) {
      const usernameMessage = setupUsernameError(username);
      if (usernameMessage) {
        setError(usernameMessage);
        return;
      }
      const passwordMessage = setupPasswordError(password);
      if (passwordMessage) {
        setError(passwordMessage);
        return;
      }
      if (password !== confirmPassword) {
        setError("passwords do not match");
        return;
      }
    }
    setSubmitting(true);

    try {
      const result = setupRequired
        ? await setupAdmin({
            setup_token: setupToken,
            username,
            password,
            display_name: displayName
          })
        : await login(username, password);
      setToken(result.session_token);
      onAuthenticated(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "authentication failed");
    } finally {
      setSubmitting(false);
    }
  }

  const setupRequired = status?.setup_required ?? false;
  const canSubmit = setupRequired
    ? setupToken.trim() !== "" &&
      username.trim() !== "" &&
      displayName.trim() !== "" &&
      password !== "" &&
      confirmPassword !== ""
    : username.trim() !== "" && password !== "";

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

        {status ? (
          <>
            {setupRequired ? (
              <div className="space-y-2">
                <Label htmlFor="setup-token">Setup token</Label>
                <Input
                  id="setup-token"
                  autoFocus
                  value={setupToken}
                  onChange={(event) => setSetupToken(event.target.value)}
                  type="password"
                  autoComplete="one-time-code"
                />
              </div>
            ) : null}

            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                autoFocus={!setupRequired}
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
              />
            </div>

            {setupRequired ? (
              <div className="space-y-2">
                <Label htmlFor="display-name">Display name</Label>
                <Input
                  id="display-name"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  autoComplete="name"
                />
              </div>
            ) : null}

            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                type="password"
                autoComplete={setupRequired ? "new-password" : "current-password"}
              />
            </div>

            {setupRequired ? (
              <div className="space-y-2">
                <Label htmlFor="confirm-password">Confirm password</Label>
                <Input
                  id="confirm-password"
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  type="password"
                  autoComplete="new-password"
                />
              </div>
            ) : null}
          </>
        ) : null}

        {error ? <p className="text-sm text-destructive">{error}</p> : null}

        <Button disabled={!status || submitting || !canSubmit} className="w-full gap-2">
          {setupRequired ? <UserPlus className="h-4 w-4" /> : <LogIn className="h-4 w-4" />}
          <span>
            {submitting
              ? setupRequired
                ? "Setting up..."
                : "Logging in..."
              : setupRequired
                ? "Set up admin"
                : "Log in"}
          </span>
        </Button>
      </form>
    </main>
  );
}

function setupUsernameError(value: string): string | null {
  const username = value.trim().toLowerCase();
  if (username.length < 3 || username.length > 32) {
    return "username must be 3-32 characters";
  }
  if (!/^[a-z0-9._-]+$/.test(username)) {
    return "username may only contain lowercase letters, numbers, dots, underscores, or hyphens";
  }
  return null;
}

function setupPasswordError(value: string): string | null {
  const bytes = new TextEncoder().encode(value).length;
  if (bytes < 12) {
    return "password must be at least 12 bytes";
  }
  if (bytes > 72) {
    return "password must be no more than 72 bytes";
  }
  return null;
}
