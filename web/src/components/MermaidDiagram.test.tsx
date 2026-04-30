// @vitest-environment jsdom

import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MermaidDiagram, resetMermaidDiagramForTests } from "./MermaidDiagram";

const mermaidMock = vi.hoisted(() => ({
  initialize: vi.fn(),
  render: vi.fn(),
}));

vi.mock("mermaid", () => ({
  default: mermaidMock,
}));

beforeEach(() => {
  resetMermaidDiagramForTests();
  mermaidMock.initialize.mockClear();
  mermaidMock.render.mockReset();
});

describe("MermaidDiagram", () => {
  it("renders Mermaid source to SVG", async () => {
    mermaidMock.render.mockResolvedValue({
      svg: '<svg role="img"><text>Expected node</text></svg>',
    });

    render(<MermaidDiagram source={"flowchart LR\nA[Expected node] --> B"} />);

    await waitFor(() => {
      expect(screen.getByTestId("mermaid-diagram").querySelector("svg")).not.toBeNull();
    });
    expect(screen.getByTestId("mermaid-diagram").textContent).toContain("Expected node");
    expect(mermaidMock.initialize).toHaveBeenCalledWith(
      expect.objectContaining({
        startOnLoad: false,
        securityLevel: "strict",
      })
    );
    expect(mermaidMock.render).toHaveBeenCalledWith(
      expect.stringMatching(/^agentx-mermaid-/),
      "flowchart LR\nA[Expected node] --> B"
    );
  });

  it("shows the source when Mermaid rendering fails", async () => {
    mermaidMock.render.mockRejectedValue(new Error("parse failed"));

    render(<MermaidDiagram source={"flowchart LR\n>"} />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Mermaid render failed");
    expect(alert.textContent).toContain("parse failed");
    expect(alert.textContent).toContain("flowchart LR");
  });
});
