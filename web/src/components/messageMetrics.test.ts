import { describe, expect, it } from "vitest";
import { formatDuration, messageMetricsParts, messageWorkingLabel, workingDurationBetween } from "./messageMetrics";

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
