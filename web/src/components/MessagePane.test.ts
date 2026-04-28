import { describe, expect, it } from "vitest";
import {
  createReadOnlyAttachmentEditorController,
  imageAttachmentPreviewDialogLabel,
  isTextAttachmentPreviewSupported,
  nextImagePreviewPan,
  nextImagePreviewScale,
} from "./MessagePane";

describe("createReadOnlyAttachmentEditorController", () => {
  it("builds a readonly workspace editor controller for attachment previews", async () => {
    const controller = createReadOnlyAttachmentEditorController(
      { filename: "notes.ts" },
      "const value = 1;\n"
    );

    expect(controller.filePath).toBe("notes.ts");
    expect(controller.trimmedPath).toBe("notes.ts");
    expect(controller.fileBody).toBe("const value = 1;\n");
    expect(controller.canUseWorkspace).toBe(false);
    expect(controller.fileLoading).toBe(false);

    controller.setFileBody("mutated");
    expect(controller.fileBody).toBe("const value = 1;\n");
    await expect(controller.saveFile()).resolves.toBeUndefined();
  });
});

describe("isTextAttachmentPreviewSupported", () => {
  it("previews text attachments in the readonly editor", () => {
    expect(
      isTextAttachmentPreviewSupported({ kind: "text", content_type: "text/plain" })
    ).toBe(true);
  });

  it("does not send image attachments through the text editor", () => {
    expect(
      isTextAttachmentPreviewSupported({ kind: "image", content_type: "image/png" })
    ).toBe(false);
  });
});

describe("imageAttachmentPreviewDialogLabel", () => {
  it("uses filename and metadata for image preview modal labels", () => {
    expect(
      imageAttachmentPreviewDialogLabel({
        filename: "diagram.png",
        content_type: "image/png",
        size_bytes: 2048,
      })
    ).toEqual({
      title: "diagram.png",
      description: "image/png · 2 KB",
    });
  });
});

describe("nextImagePreviewScale", () => {
  it("zooms in and out from wheel delta", () => {
    expect(nextImagePreviewScale(1, -600)).toBeGreaterThan(1);
    expect(nextImagePreviewScale(1, 600)).toBeLessThan(1);
  });

  it("clamps zoom bounds", () => {
    expect(nextImagePreviewScale(6, -600)).toBe(6);
    expect(nextImagePreviewScale(0.25, 600)).toBe(0.25);
  });
});

describe("nextImagePreviewPan", () => {
  it("moves a zoomed image by the pointer delta", () => {
    expect(nextImagePreviewPan({ x: 10, y: -5 }, 24.25, -12.5, 2)).toEqual({
      x: 34.25,
      y: -17.5,
    });
  });

  it("resets pan when the image is not zoomed in", () => {
    expect(nextImagePreviewPan({ x: 10, y: 20 }, 5, 5, 1)).toEqual({ x: 0, y: 0 });
  });
});
