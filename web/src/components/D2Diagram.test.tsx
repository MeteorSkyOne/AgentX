// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  D2Diagram,
  cachedD2SVG,
  clearD2DiagramCacheForTests,
  nextD2PreviewScale,
} from "./D2Diagram";

const apiMock = vi.hoisted(() => ({
  renderD2Diagram: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  renderD2Diagram: apiMock.renderD2Diagram,
}));

const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;

beforeEach(() => {
  clearD2DiagramCacheForTests();
  apiMock.renderD2Diagram.mockReset();
  URL.createObjectURL = vi.fn(() => "blob:agentx-d2");
  URL.revokeObjectURL = vi.fn();
});

afterEach(() => {
  cleanup();
  clearD2DiagramCacheForTests();
  URL.createObjectURL = originalCreateObjectURL;
  URL.revokeObjectURL = originalRevokeObjectURL;
});

describe("D2Diagram", () => {
  it("renders D2 source through the backend API", async () => {
    apiMock.renderD2Diagram.mockResolvedValue({
      svg: '<svg xmlns="http://www.w3.org/2000/svg"><text>D2 node</text></svg>',
      cached: false,
    });

    render(<D2Diagram source="x -> y" />);

    const image = await screen.findByTestId("d2-diagram-image");
    expect(image.getAttribute("src")).toBe("blob:agentx-d2");
    expect(apiMock.renderD2Diagram).toHaveBeenCalledWith("x -> y");
  });

  it("constrains rendered diagrams to a bounded viewport", async () => {
    apiMock.renderD2Diagram.mockResolvedValue({
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="4000" height="4000"><text>D2 node</text></svg>',
      cached: false,
    });

    render(<D2Diagram source="large -> diagram" />);

    const viewport = await screen.findByTestId("d2-diagram-viewport");
    const image = screen.getByTestId("d2-diagram-image");
    expect(viewport.style.maxHeight).toBe("min(60svh, 32rem)");
    expect(image.style.maxHeight).toBe("");
    expect(image.className).toContain("max-w-none");
    expect(screen.queryByRole("button", { name: "Fit diagram to width" })).toBeNull();
  });

  it("opens a draggable preview with zoom controls", async () => {
    apiMock.renderD2Diagram.mockResolvedValue({
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="800"><text>D2 node</text></svg>',
      cached: false,
    });

    render(<D2Diagram source="large -> diagram" />);

    fireEvent.click(await screen.findByTestId("d2-diagram-image"));
    expect(await screen.findByRole("dialog", { name: "D2 Diagram" })).toBeTruthy();

    const viewport = screen.getByTestId("d2-diagram-preview-viewport");
    const canvas = screen.getByTestId("d2-diagram-preview-canvas");
    Object.defineProperty(viewport, "clientWidth", { configurable: true, value: 500 });
    Object.defineProperty(viewport, "clientHeight", { configurable: true, value: 400 });
    const previewImage = screen.getByTestId("d2-diagram-preview-image") as HTMLImageElement;
    Object.defineProperty(previewImage, "naturalWidth", { configurable: true, value: 1000 });
    Object.defineProperty(previewImage, "naturalHeight", { configurable: true, value: 800 });
    fireEvent.load(previewImage);

    await waitFor(() => expect(previewImage.style.width).toBe("1000px"));
    expect(viewport.className).toContain("overflow-hidden");
    expect(viewport.className).toContain("cursor-grab");
    expect(viewport.className).toContain("touch-none");
    expect(canvas.className).toContain("relative");
    expect(previewImage.style.transform).toContain("translate(0px, 0px)");

    fireEvent.click(screen.getByRole("button", { name: "Zoom in D2 diagram" }));
    await waitFor(() => expect(previewImage.style.width).toBe("1250px"));

    fireEvent.click(screen.getByRole("button", { name: "Zoom out D2 diagram" }));
    await waitFor(() => expect(previewImage.style.width).toBe("1000px"));

    fireEvent.wheel(viewport, { deltaY: -300 });
    await waitFor(() => expect(Number.parseInt(previewImage.style.width, 10)).toBeGreaterThan(1000));

    fireEvent.pointerDown(viewport, {
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerMove(viewport, {
      pointerId: 1,
      pointerType: "mouse",
      clientX: 180,
      clientY: 140,
    });
    await waitFor(() => expect(previewImage.style.transform).toContain("translate(80px, 40px)"));
    fireEvent.pointerUp(viewport, { pointerId: 1, pointerType: "mouse" });
  });

  it("clamps D2 preview wheel zoom", () => {
    expect(nextD2PreviewScale(1, -1000)).toBeGreaterThan(1);
    expect(nextD2PreviewScale(4, -1000)).toBeGreaterThan(4);
    expect(nextD2PreviewScale(50, -1000)).toBe(50);
    expect(nextD2PreviewScale(0.25, 1000)).toBe(0.25);
  });

  it("caches repeated D2 source in the frontend", async () => {
    apiMock.renderD2Diagram.mockResolvedValue({
      svg: '<svg xmlns="http://www.w3.org/2000/svg"><text>D2 node</text></svg>',
      cached: false,
    });

    const first = render(<D2Diagram source="cached -> graph" />);
    await screen.findByTestId("d2-diagram-image");
    first.unmount();

    render(<D2Diagram source="cached -> graph" />);
    await screen.findByTestId("d2-diagram-image");

    expect(apiMock.renderD2Diagram).toHaveBeenCalledTimes(1);
  });

  it("shares in-flight requests for the same source", async () => {
    let resolveRender: (value: { svg: string; cached: boolean }) => void = () => undefined;
    apiMock.renderD2Diagram.mockReturnValue(
      new Promise((resolve) => {
        resolveRender = resolve;
      })
    );

    const first = cachedD2SVG("same -> source");
    const second = cachedD2SVG("same -> source");
    await waitFor(() => expect(apiMock.renderD2Diagram).toHaveBeenCalledTimes(1));

    resolveRender({
      svg: '<svg xmlns="http://www.w3.org/2000/svg"><text>shared</text></svg>',
      cached: false,
    });

    await expect(first).resolves.toContain("shared");
    await expect(second).resolves.toContain("shared");
  });

  it("shows source when D2 rendering fails", async () => {
    apiMock.renderD2Diagram.mockRejectedValue(new Error("D2 parse failed"));

    render(<D2Diagram source="x ->" />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("D2 render failed");
    expect(alert.textContent).toContain("D2 parse failed");
    expect(alert.textContent).toContain("x ->");
  });
});
