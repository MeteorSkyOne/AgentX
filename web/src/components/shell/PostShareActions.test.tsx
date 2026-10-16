// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { exportThreadMarkdown } from "../../api/client";
import type { Thread } from "../../api/types";
import { PostShareActions, postExportFilename } from "./PostShareActions";

vi.mock("../../api/client", () => ({
  exportThreadMarkdown: vi.fn(async () => new Blob(["# post"], { type: "text/markdown" }))
}));

afterEach(cleanup);

describe("post export filename", () => {
  it("keeps the title readable and always includes the post ID", () => {
    expect(postExportFilename({ id: "thr_abc", title: "发布计划 v2" })).toBe("发布计划-v2-thr_abc.md");
    expect(postExportFilename({ id: "thr_abc", title: 'a/b:c*"d"' })).toBe("a-b-c-d-thr_abc.md");
    expect(postExportFilename({ id: "thr_abc", title: "  " })).toBe("thr_abc.md");
  });
});

describe("post export dialog", () => {
  it("passes the include-process choice to the export request", async () => {
    URL.createObjectURL = vi.fn(() => "blob:post");
    URL.revokeObjectURL = vi.fn();
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    const thread: Thread = {
      id: "thr_abc",
      organization_id: "org_1",
      project_id: "prj_1",
      channel_id: "chn_1",
      title: "Release plan",
      created_by: "usr_1",
      created_at: "2026-10-16T00:00:00Z",
      updated_at: "2026-10-16T00:00:00Z"
    };
    render(<PostShareActions thread={thread} />);

    fireEvent.click(screen.getByRole("button", { name: "Export post as Markdown" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Include thinking and tool calls" }));
    fireEvent.click(screen.getByRole("button", { name: "Export" }));

    await waitFor(() => expect(clickSpy).toHaveBeenCalled());
    expect(vi.mocked(exportThreadMarkdown)).toHaveBeenCalledWith("thr_abc", { includeProcess: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    clickSpy.mockRestore();
  });
});
