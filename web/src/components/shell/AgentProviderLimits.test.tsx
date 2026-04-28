// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentProviderLimits } from "../../api/types";
import {
  AgentProviderLimitsView,
  formatProviderLimitPercent,
  formatProviderLimitReset,
} from "./AgentProviderLimits";
import { isProviderLimitAgent } from "./utils";

afterEach(cleanup);

describe("AgentProviderLimitsView", () => {
  it("renders Codex limit windows", () => {
    render(
      <AgentProviderLimitsView
        limits={codexLimits()}
        isLoading={false}
        isFetching={false}
        onRefresh={() => undefined}
      />
    );

    expect(screen.getByText("Usage limits")).toBeTruthy();
    expect(screen.getByText("Live")).toBeTruthy();
    expect(screen.getByText("5-hour")).toBeTruthy();
    expect(screen.getByText("Weekly")).toBeTruthy();
    expect(screen.getByText("43%")).toBeTruthy();
    expect(screen.getByText("10%")).toBeTruthy();
  });

  it("renders Claude unavailable state", () => {
    render(
      <AgentProviderLimitsView
        limits={claudeUnavailableLimits()}
        isLoading={false}
        isFetching={false}
        onRefresh={() => undefined}
      />
    );

    expect(screen.getByText("Unavailable")).toBeTruthy();
    expect(screen.getByText(/Signed in/).textContent).toContain("max");
    expect(screen.getByText(/OAuth credentials were not found/)).toBeTruthy();
  });

  it("renders loading and error states", () => {
    const { rerender } = render(
      <AgentProviderLimitsView
        isLoading={true}
        isFetching={true}
        onRefresh={() => undefined}
      />
    );
    expect(screen.getByText("Loading usage limits...")).toBeTruthy();
    expect((screen.getByLabelText("Refresh usage limits") as HTMLButtonElement).disabled).toBe(true);

    rerender(
      <AgentProviderLimitsView
        error={new Error("probe failed")}
        isLoading={false}
        isFetching={false}
        onRefresh={() => undefined}
      />
    );
    expect(screen.getByText("Error")).toBeTruthy();
    expect(screen.getByText("probe failed")).toBeTruthy();
  });

  it("calls refetch from the refresh button", () => {
    const refetch = vi.fn();
    render(
      <AgentProviderLimitsView
        limits={codexLimits()}
        isLoading={false}
        isFetching={false}
        onRefresh={refetch}
      />
    );

    fireEvent.click(screen.getByLabelText("Refresh usage limits"));
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

describe("provider limit helpers", () => {
  it("formats percentages and reset durations", () => {
    expect(formatProviderLimitPercent(42.5)).toBe("43%");
    expect(formatProviderLimitPercent(null)).toBe("unknown");
    expect(
      formatProviderLimitReset(
        "2026-04-28T14:30:00Z",
        new Date("2026-04-28T12:00:00Z")
      )
    ).toBe("resets in 2h 30m");
  });

  it("detects supported provider limit agents", () => {
    expect(isProviderLimitAgent("codex")).toBe(true);
    expect(isProviderLimitAgent("claude")).toBe(true);
    expect(isProviderLimitAgent("fake")).toBe(false);
    expect(isProviderLimitAgent(undefined)).toBe(false);
  });
});

function codexLimits(): AgentProviderLimits {
  return {
    agent_id: "agt_codex",
    provider: "codex",
    status: "ok",
    auth: {
      logged_in: true,
      method: "chatgpt",
      provider: "openai",
      plan: "plus",
    },
    windows: [
      {
        kind: "five_hour",
        label: "5-hour",
        used_percent: 42.5,
        window_minutes: 300,
        resets_at: "2026-04-28T14:00:00Z",
      },
      {
        kind: "seven_day",
        label: "Weekly",
        used_percent: 10,
        window_minutes: 10080,
        resets_at: "2026-05-05T12:00:00Z",
      },
    ],
    fetched_at: "2026-04-28T12:00:00Z",
    cache_ttl_seconds: 45,
  };
}

function claudeUnavailableLimits(): AgentProviderLimits {
  return {
    agent_id: "agt_claude",
    provider: "claude",
    status: "unavailable",
    auth: {
      logged_in: true,
      method: "oauth",
      provider: "claude.ai",
      plan: "max",
    },
    windows: [],
    fetched_at: "2026-04-28T12:00:00Z",
    cache_ttl_seconds: 45,
    message: "Claude Code OAuth credentials were not found; numeric usage limits are unavailable.",
  };
}
