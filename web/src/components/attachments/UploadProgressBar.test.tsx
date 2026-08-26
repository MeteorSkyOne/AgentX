// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { UploadProgressBar } from "./UploadProgressBar";

afterEach(cleanup);

describe("UploadProgressBar", () => {
  it("renders nothing when no upload is in flight", () => {
    const { container } = render(<UploadProgressBar progress={null} />);
    expect(container.firstChild).toBeNull();
  });

  it("shows transferred bytes and percent while uploading", () => {
    render(
      <UploadProgressBar progress={{ loaded: 512 * 1024, total: 2 * 1024 * 1024, percent: 25 }} />
    );

    expect(screen.getByText("Uploading 512 KB of 2.0 MB")).toBeTruthy();
    expect(screen.getByText("25%")).toBeTruthy();
    const bar = screen.getByRole("progressbar");
    expect(bar.getAttribute("aria-valuenow")).toBe("25");
  });

  it("switches to a processing label once the body is fully on the wire", () => {
    render(<UploadProgressBar progress={{ loaded: 1000, total: 1000, percent: 100 }} />);

    expect(screen.getByText("Processing upload…")).toBeTruthy();
    expect(screen.queryByText("100%")).toBeNull();
  });

  it("stays indeterminate when the total size is unknown", () => {
    render(<UploadProgressBar progress={{ loaded: 4096, total: 0, percent: 0 }} />);

    expect(screen.getByText("Uploading…")).toBeTruthy();
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBeNull();
  });
});
