import { FormEvent, useState } from "react";
import { LogIn } from "lucide-react";
import { bootstrap, setToken } from "../api/client";
import type { BootstrapResponse } from "../api/types";

interface LoginViewProps {
  onBootstrap: (result: BootstrapResponse) => void;
}

export function LoginView({ onBootstrap }: LoginViewProps) {
  const [adminToken, setAdminToken] = useState("");
  const [displayName, setDisplayName] = useState("Admin");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const result = await bootstrap(adminToken, displayName);
      setToken(result.session_token);
      onBootstrap(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Bootstrap failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-screen">
      <form className="login-panel" onSubmit={handleSubmit}>
        <div className="login-heading">
          <span className="product-mark">AX</span>
          <div>
            <h1>AgentX</h1>
            <p>Foundation workspace</p>
          </div>
        </div>

        <label className="field">
          <span>Admin token</span>
          <input
            autoFocus
            value={adminToken}
            onChange={(event) => setAdminToken(event.target.value)}
            type="password"
            autoComplete="current-password"
          />
        </label>

        <label className="field">
          <span>Display name</span>
          <input
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            autoComplete="name"
          />
        </label>

        {error ? <p className="form-error">{error}</p> : null}

        <button className="primary-button" disabled={submitting || adminToken.trim() === ""}>
          <LogIn size={18} />
          <span>{submitting ? "Bootstrapping" : "Enter"}</span>
        </button>
      </form>
    </main>
  );
}
