// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import { MarkdownRenderer } from "./MarkdownRenderer";

const mountedRoots: Array<{ root: Root; container: HTMLDivElement }> = [];

beforeAll(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
});

afterEach(() => {
  for (const { root, container } of mountedRoots.splice(0)) {
    act(() => root.unmount());
    container.remove();
  }
});

describe("MarkdownRenderer", () => {
  it("renders inline and block math with KaTeX markup", () => {
    const html = renderToStaticMarkup(
      <MarkdownRenderer text={"Inline $E = mc^2$.\n\n$$\na^2 + b^2 = c^2\n$$"} />
    );

    expect(html).toContain("katex");
    expect(html).toContain("katex-display");
    expect(html).toContain("aria-hidden=\"true\"");
  });

  it("renders plain text workspace paths as clickable controls", () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = renderClient(
      <MarkdownRenderer
        text="Open web/src/App.tsx:12:3."
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={(target) => opened.push(target)}
      />
    );

    const button = workspacePathButton(container);
    expect(button.textContent).toBe("web/src/App.tsx:12:3");
    click(button);

    expect(opened).toEqual([
      {
        path: "web/src/App.tsx",
        lineNumber: 12,
        column: 3,
        label: "web/src/App.tsx:12:3",
      },
    ]);
    expect(container.textContent).toBe("Open web/src/App.tsx:12:3.");
  });

  it("leaves workspace paths as text when workspace navigation is unavailable", () => {
    const { container } = renderClient(
      <MarkdownRenderer
        text="Open web/src/App.tsx:12:3."
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(workspacePathButtons(container)).toEqual([]);
    expect(container.textContent).toBe("Open web/src/App.tsx:12:3.");
  });

  it("renders multiple text paths in one paragraph", () => {
    const { container } = renderClient(
      <MarkdownRenderer
        text="Open web/src/App.tsx and internal/app/app.go"
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(workspacePathButtons(container).map((button) => button.textContent)).toEqual([
      "web/src/App.tsx",
      "internal/app/app.go",
    ]);
    expect(container.textContent).toBe("Open web/src/App.tsx and internal/app/app.go");
  });

  it("renders a single inline code path as clickable", () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = renderClient(
      <MarkdownRenderer
        text="`web/src/App.tsx:8`"
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={(target) => opened.push(target)}
      />
    );

    const button = workspacePathButton(container);
    expect(button.querySelector("code")?.textContent).toBe("web/src/App.tsx:8");
    click(button);

    expect(opened).toEqual([
      {
        path: "web/src/App.tsx",
        lineNumber: 8,
        column: 1,
        label: "web/src/App.tsx:8",
      },
    ]);
  });

  it("does not parse fenced code block paths", () => {
    const html = renderToStaticMarkup(
      <MarkdownRenderer
        text={"```tsx\nweb/src/App.tsx:8\n```"}
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(html).not.toContain("data-workspace-path");
  });

  it("keeps external markdown links opening in a new tab", () => {
    const html = renderToStaticMarkup(
      <MarkdownRenderer
        text="[docs](https://example.com/docs)"
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(html).toContain('href="https://example.com/docs"');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer"');
  });

  it("renders paths in list items and table cells", () => {
    const { container } = renderClient(
      <MarkdownRenderer
        text={"- web/src/App.tsx\n\n| File |\n| --- |\n| internal/app/app.go |"}
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(workspacePathButtons(container).map((button) => button.textContent)).toEqual([
      "web/src/App.tsx",
      "internal/app/app.go",
    ]);
  });

  it("opens markdown relative links as workspace paths", () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = renderClient(
      <MarkdownRenderer
        text="[App file](web/src/App.tsx:7)"
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={(target) => opened.push(target)}
      />
    );

    const button = workspacePathButton(container);
    expect(button.textContent).toBe("App file");
    click(button);

    expect(opened).toEqual([
      {
        path: "web/src/App.tsx",
        lineNumber: 7,
        column: 1,
        label: "web/src/App.tsx:7",
      },
    ]);
  });

  it("resolves markdown file links relative to the current markdown file", () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = renderClient(
      <MarkdownRenderer
        text="[Next](next.md) and [Guide](../guide.md:4)"
        workspacePath="/workspace/AgentX"
        relativeLinkBasePath="docs/chapter/readme.md"
        onOpenWorkspacePath={(target) => opened.push(target)}
      />
    );

    const buttons = workspacePathButtons(container);
    expect(buttons.map((button) => button.textContent)).toEqual(["Next", "Guide"]);

    click(buttons[0]);
    click(buttons[1]);

    expect(opened).toEqual([
      {
        path: "docs/chapter/next.md",
        label: "docs/chapter/next.md",
      },
      {
        path: "docs/guide.md",
        lineNumber: 4,
        column: 1,
        label: "docs/guide.md:4",
      },
    ]);
  });

  it("does not resolve markdown links that escape the workspace root", () => {
    const { container } = renderClient(
      <MarkdownRenderer
        text="[Secret](../secret.md)"
        workspacePath="/workspace/AgentX"
        relativeLinkBasePath="README.md"
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(workspacePathButtons(container)).toEqual([]);
    expect(container.textContent).toBe("Secret");
  });

  it("renders invalid markdown workspace links as plain text", () => {
    const { container } = renderClient(
      <MarkdownRenderer
        text="[not a file](internal/app)"
        workspacePath="/workspace/AgentX"
        onOpenWorkspacePath={vi.fn()}
      />
    );

    expect(workspacePathButtons(container)).toEqual([]);
    expect(container.textContent).toBe("not a file");
  });
});

function renderClient(element: ReactElement): { container: HTMLDivElement } {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ root, container });
  act(() => {
    root.render(element);
  });
  return { container };
}

function workspacePathButton(container: HTMLElement): HTMLButtonElement {
  const button = workspacePathButtons(container)[0];
  expect(button).not.toBeNull();
  return button as HTMLButtonElement;
}

function workspacePathButtons(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button[data-workspace-path]"));
}

function click(element: HTMLElement) {
  act(() => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
  });
}
