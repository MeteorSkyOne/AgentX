import { describe, expect, it } from "vitest";
import {
  contextUsageLabel,
  formatDuration,
  formatTokenCount,
  messageMetricsParts,
  messageWorkingLabel,
  workingDurationBetween,
} from "./messageMetrics";

describe("messageMetricsParts", () => {
  it("honors TTFT and TPS preferences independently", () => {
    const metrics = {
      run_id: "run_1",
      provider: "codex",
      ttft_ms: 250,
      tps: 12.34,
    };

    expect(messageMetricsParts(metrics, { show_ttft: true, show_tps: true })).toEqual([
      "TTFT 250ms",
      "TPS 12.3",
    ]);
    expect(messageMetricsParts(metrics, { show_ttft: false, show_tps: true })).toEqual([
      "TPS 12.3",
    ]);
    expect(messageMetricsParts(metrics, { show_ttft: true, show_tps: false })).toEqual([
      "TTFT 250ms",
    ]);
  });

  it("formats working durations across seconds, minutes, and hours", () => {
    expect(formatDuration(1200)).toBe("1.2s");
    expect(formatDuration(12_400)).toBe("12s");
    expect(formatDuration(62_000)).toBe("1m 2s");
    expect(formatDuration(3_723_000)).toBe("1h 2m 3s");
  });

  it("builds working labels from metric duration or timestamps", () => {
    expect(messageWorkingLabel({ run_id: "run_1", provider: "codex", duration_ms: 1200 })).toBe(
      "Working 1.2s"
    );
    expect(
      messageWorkingLabel({
        run_id: "run_1",
        provider: "codex",
        started_at: "2026-04-20T20:47:00Z",
        completed_at: "2026-04-20T20:48:02Z",
      })
    ).toBe("Working 1m 2s");
    expect(
      workingDurationBetween("2026-04-20T20:47:00Z", undefined, new Date("2026-04-20T20:47:01.200Z"))
    ).toBe("Working 1.2s");
  });
});

describe("contextUsageLabel", () => {
  it("shows used over window with a percentage", () => {
    expect(
      contextUsageLabel({ total_tokens: 76_420, context_window_tokens: 200_000, used_percent: 38.21 })
    ).toBe("Context 76k/200k (38%)");
  });

  it("derives the percentage when the agent only reports counts", () => {
    expect(contextUsageLabel({ total_tokens: 100_000, context_window_tokens: 1_000_000 })).toBe(
      "Context 100k/1m (10%)"
    );
  });

  it("degrades to whatever fields are present", () => {
    expect(contextUsageLabel({ total_tokens: 512 })).toBe("Context 512");
    expect(contextUsageLabel({ used_percent: 42 })).toBe("Context 42%");
    expect(contextUsageLabel({})).toBeNull();
    expect(contextUsageLabel(undefined)).toBeNull();
  });

  it("formats token counts in k and m", () => {
    expect(formatTokenCount(999)).toBe("999");
    expect(formatTokenCount(1_500)).toBe("2k");
    expect(formatTokenCount(200_000)).toBe("200k");
    expect(formatTokenCount(1_250_000)).toBe("1.3m");
    expect(formatTokenCount(1_000_000)).toBe("1m");
  });
});
