import { describe, expect, it } from "vitest";
import { messageMetricsParts } from "./messageMetrics";

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
});
