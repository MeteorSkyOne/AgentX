// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CodeBlock } from "./CodeBlock";

vi.mock("./MermaidDiagram", () => ({
  MermaidDiagram: ({ source }: { source: string }) => (
    <div data-testid="mock-mermaid">{source}</div>
  ),
}));

vi.mock("./D2Diagram", () => ({
  D2Diagram: ({ source }: { source: string }) => (
    <div data-testid="mock-d2">{source}</div>
  ),
}));

afterEach(() => {
  cleanup();
});

describe("CodeBlock", () => {
  it("routes mermaid fenced code to the Mermaid renderer", () => {
    render(<CodeBlock className="language-mermaid">graph TD; A--&gt;B{"\n"}</CodeBlock>);

    expect(screen.getByTestId("mock-mermaid").textContent).toBe("graph TD; A-->B");
    expect(screen.queryByTestId("code-block")).toBeNull();
  });

  it("routes mmd fenced code to the Mermaid renderer", () => {
    render(<CodeBlock className="language-mmd">flowchart LR{"\n"}</CodeBlock>);

    expect(screen.getByTestId("mock-mermaid").textContent).toBe("flowchart LR");
  });

  it("routes d2 fenced code to the D2 renderer", () => {
    render(<CodeBlock className="language-d2">x -&gt; y{"\n"}</CodeBlock>);

    expect(screen.getByTestId("mock-d2").textContent).toBe("x -> y");
    expect(screen.queryByTestId("code-block")).toBeNull();
  });

  it("keeps normal fenced code as highlighted code", () => {
    render(<CodeBlock className="language-ts">const x = 1;{"\n"}</CodeBlock>);

    expect(screen.getByTestId("code-block")).not.toBeNull();
    expect(screen.queryByTestId("mock-mermaid")).toBeNull();
    expect(screen.queryByTestId("mock-d2")).toBeNull();
  });
});
