import { describe, expect, it } from "vitest";
import {
  AGENT_EFFORT_OPTIONS,
  AGENT_RUNTIME_OPTIONS,
  agentEffortOptions,
  agentKindFromProviderPersistent,
  agentKindLabel,
  agentKindSupportsFastMode,
  agentPersistentFromKind,
  agentProviderFromKind,
  agentProviderHasPersistentToggle,
  conversationActivityLabel,
  defaultAgentInstructionPath,
  isProviderLimitAgent,
} from "./utils";

describe("conversation activity label", () => {
  it("only reports ready when the socket is connected and idle", () => {
    expect(conversationActivityLabel("connected", false)).toBe("ready");
    expect(conversationActivityLabel("connected", true)).toBe("running");
    expect(conversationActivityLabel("connecting", false)).toBe("connecting");
    expect(conversationActivityLabel("reconnecting", false)).toBe("reconnecting");
    expect(conversationActivityLabel("disconnected", false)).toBe("offline");
    expect(conversationActivityLabel("idle", false)).toBe("offline");
  });
});

describe("antigravity (agy) runtime helpers", () => {
  it("maps agy as an always-persistent runtime without fast mode", () => {
    expect(AGENT_RUNTIME_OPTIONS.some((item) => item.value === "agy" && item.label === "Antigravity")).toBe(true);
    expect(agentProviderFromKind("agy")).toBe("agy");
    expect(agentPersistentFromKind("agy")).toBe(true);
    expect(agentProviderHasPersistentToggle("agy")).toBe(false);
    expect(agentKindFromProviderPersistent("agy", false)).toBe("agy");
    expect(agentKindFromProviderPersistent("agy", true)).toBe("agy");
    expect(agentKindSupportsFastMode("agy")).toBe(false);
    expect(agentEffortOptions("agy")).toBe(AGENT_EFFORT_OPTIONS);
    expect(agentKindLabel("agy")).toBe("Antigravity");
    expect(defaultAgentInstructionPath("agy")).toBe("GEMINI.md");
    expect(isProviderLimitAgent("agy")).toBe(true);
  });
});
