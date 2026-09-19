import type { ToolUpdateStatus } from "@/api/types";

export function formatToolVersion(tool?: ToolUpdateStatus): string {
  const current = tool?.current_version || toolVersionPlaceholder(tool?.state);
  return tool?.latest_version ? `${current} -> ${tool.latest_version}` : current;
}

function toolVersionPlaceholder(state?: string): string {
  if (state === "checking" || state === "updating") return "checking";
  if (state === "error") return "unavailable";
  return "not checked";
}
