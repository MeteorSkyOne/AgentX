// @vitest-environment jsdom

import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { pageURLForVersion, useReloadOnServerVersionChange } from "./useReloadOnServerVersionChange";

describe("pageURLForVersion", () => {
  it("preserves the current location and adds a version cache buster", () => {
    expect(pageURLForVersion("https://agentx.example/?view=chat#message", "1.1.0+dev")).toBe(
      "https://agentx.example/?view=chat&_agentx_version=1.1.0%2Bdev#message"
    );
  });
});

describe("useReloadOnServerVersionChange", () => {
  it("reloads when the server reports a different version", () => {
    const reload = vi.fn();
    const { rerender } = renderHook(
      ({ version }) => useReloadOnServerVersionChange(version, reload),
      { initialProps: { version: "1.0.0" } }
    );

    rerender({ version: "1.0.0" });
    expect(reload).not.toHaveBeenCalled();

    rerender({ version: "1.1.0" });
    expect(reload).toHaveBeenCalledOnce();
    expect(reload).toHaveBeenCalledWith("1.1.0");
  });

  it("uses the first available version as the baseline", () => {
    const reload = vi.fn();
    const { rerender } = renderHook(
      ({ version }: { version?: string }) => useReloadOnServerVersionChange(version, reload),
      { initialProps: { version: undefined as string | undefined } }
    );

    rerender({ version: "1.0.0" });
    expect(reload).not.toHaveBeenCalled();
  });
});
