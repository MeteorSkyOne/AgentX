import { describe, expect, it } from "vitest";
import { isAcceptedDraftAttachment, selectDraftAttachmentFiles } from "./Composer";

describe("selectDraftAttachmentFiles", () => {
  it("keeps valid files when another dropped file is unsupported", () => {
    const image = new File([new Uint8Array([1, 2, 3])], "screen.png", { type: "image/png" });
    const binary = new File([new Uint8Array([1, 2, 3])], "tool.exe", {
      type: "application/octet-stream"
    });

    const result = selectDraftAttachmentFiles([], [image, binary]);

    expect(result.accepted).toEqual([image]);
    expect(result.rejected).toEqual(["tool.exe is not a supported attachment type"]);
  });

  it("accepts source files selected by extension when the browser omits MIME type", () => {
    const source = new File(["package main"], "main.go", { type: "" });

    expect(isAcceptedDraftAttachment(source)).toBe(true);
  });

  it("rejects files beyond the per-message attachment limit without dropping available slots", () => {
    const existing = Array.from({ length: 4 }, (_, index) =>
      new File(["x"], `existing-${index}.txt`, { type: "text/plain" })
    );
    const first = new File(["x"], "first.txt", { type: "text/plain" });
    const second = new File(["x"], "second.txt", { type: "text/plain" });

    const result = selectDraftAttachmentFiles(existing, [first, second]);

    expect(result.accepted).toEqual([first]);
    expect(result.rejected).toEqual(["second.txt exceeds the 5 file limit"]);
  });
});
