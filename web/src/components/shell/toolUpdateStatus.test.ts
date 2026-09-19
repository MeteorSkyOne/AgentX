import { describe, expect, it } from "vitest";
import type { ToolUpdateStatus } from "@/api/types";
import { formatToolVersion } from "./toolUpdateStatus";

function status(overrides: Partial<ToolUpdateStatus>): ToolUpdateStatus {
  return {
    tool: "codex",
    display_name: "Codex",
    command: "codex",
    state: "idle",
    active_run_count: 0,
    runtime_reset_pending: false,
    ...overrides
  };
}

describe("formatToolVersion", () => {
  it("shows installed and latest versions", () => {
    expect(formatToolVersion(status({ current_version: "0.153.0", latest_version: "0.154.0" }))).toBe("0.153.0 -> 0.154.0");
  });

  it("describes a missing version by check state", () => {
    expect(formatToolVersion(status({ state: "checking" }))).toBe("checking");
    expect(formatToolVersion(status({ state: "error" }))).toBe("unavailable");
    expect(formatToolVersion(status({}))).toBe("not checked");
    expect(formatToolVersion(undefined)).toBe("not checked");
  });
});
