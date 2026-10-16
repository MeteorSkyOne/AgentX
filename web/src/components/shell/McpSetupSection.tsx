import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Copy } from "lucide-react";
import { mcpSetup } from "@/api/client";
import { Button } from "@/components/ui/button";
import { copyTextToClipboard } from "../message-pane/utils";

/** Shows how to connect local AI CLIs to the read-only post lookup MCP server. */
export function McpSetupSection({ enabled }: { enabled: boolean }) {
  const setupQuery = useQuery({
    queryKey: ["mcp-setup"],
    queryFn: mcpSetup,
    enabled,
    staleTime: Infinity
  });
  const setup = setupQuery.data;

  return (
    <div className="grid gap-3 rounded-md border border-border p-3">
      <div>
        <h3 className="text-sm font-medium">Post lookup MCP</h3>
        <p className="text-xs text-muted-foreground">
          Lets local AI agents read a post by its copied ID (tools: get_post, search_post) without
          inheriting that post's agent session. Read-only, runs over stdio on this machine.
        </p>
      </div>
      {setupQuery.isLoading && <p className="text-xs text-muted-foreground">Loading</p>}
      {setupQuery.error && (
        <p className="text-sm text-destructive">
          {setupQuery.error instanceof Error ? setupQuery.error.message : "Load MCP setup failed"}
        </p>
      )}
      {setup && (
        <>
          {!setup.available && (
            <p className="text-xs text-muted-foreground">
              This server runs from a temporary build; replace <code>agentx</code> with the path to a
              built AgentX binary.
            </p>
          )}
          <SetupCommand label="Claude Code" command={setup.claude_command} />
          <SetupCommand label="Codex" command={setup.codex_command} />
        </>
      )}
    </div>
  );
}

function SetupCommand({ label, command }: { label: string; command: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    await copyTextToClipboard(command);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="grid gap-1">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          title={`Copy ${label} command`}
          aria-label={`Copy ${label} command`}
          onClick={() => void copy()}
        >
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        </Button>
      </div>
      <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
        {command}
      </pre>
    </div>
  );
}
