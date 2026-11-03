// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, onTestFinished, vi } from "vitest";
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
  function mathMarkup(text: string) {
    const container = document.createElement("div");
    container.innerHTML = renderToStaticMarkup(<MarkdownRenderer text={text} />);
    return container;
  }

  it("renders LaTeX delimiters from agent replies without parsing formula content as Markdown", () => {
    const container = mathMarkup(String.raw`MLP 输出：

\[
y=Wa=\sum_j W_{:,j}a_j
\]

- \(a_j\)：第 \(j\) 个神经元。

\[
s_j=
\underbrace{\left(\bar a_j^{重复}-\bar a_j^{正常}\right)}_{\text{重复时激活增加多少}}
\cdot
\underbrace{\left(u^\top W_{:,j}\right)}_{\text{输出有多少沿着重复方向}}
\]`);
    expect(container.querySelectorAll(".katex")).toHaveLength(4);
    expect(container.querySelectorAll(".katex-display")).toHaveLength(2);
    expect(container.querySelector(".katex-error")).toBeNull();
    expect(container.querySelector("em")).toBeNull();
    expect(container.querySelector("annotation")?.textContent?.trim()).toBe(String.raw`y=Wa=\sum_j W_{:,j}a_j`);
  });

  it("supports LaTeX math in quotes and lists alongside dollar math", () => {
    const container = mathMarkup(String.raw`> \[
> x_1 + x_2
> \]

- \(a_b\), $c_d$, and \[e_f\].

$$
g_h
$$`);
    expect(container.querySelectorAll(".katex")).toHaveLength(5);
    expect(container.querySelectorAll(".katex-display")).toHaveLength(3);
    expect(container.querySelector(".katex-error")).toBeNull();
    expect(container.querySelector("blockquote annotation")?.textContent?.trim()).toBe("x_1 + x_2");
  });

  it("leaves LaTeX delimiters inside code and escaped delimiters as literal text", () => {
    const container = mathMarkup([
      "`" + String.raw`\(a_b\) \[c_d\]` + "`",
      "",
      "```text",
      String.raw`\[x_y\]`,
      "```",
      "",
      "    " + String.raw`\(z_w\)`,
      "",
      String.raw`\\(literal\\) and \\[literal\\]`,
    ].join("\n"));
    expect(container.querySelector(".katex")).toBeNull();
    expect(container.textContent).toContain(String.raw`\(a_b\) \[c_d\]`);
    expect(container.textContent).toContain(String.raw`\[x_y\]`);
    expect(container.textContent).toContain(String.raw`\(z_w\)`);
  });

  it("keeps unclosed LaTeX math as text until the closing delimiter arrives", () => {
    const partial = String.raw`Formula: \(a_b`;
    expect(mathMarkup(partial).querySelector(".katex")).toBeNull();
    expect(mathMarkup(partial + String.raw`\)`).querySelector(".katex")).not.toBeNull();
    expect(mathMarkup(String.raw`\[x_y\)`).querySelector(".katex")).toBeNull();
  });

  it("preserves TeX line breaks and dollar signs inside LaTeX delimiters", () => {
    const formula = String.raw`\begin{aligned}a &= b \\ c &= \text{cost: \$5}\end{aligned}`;
    const container = mathMarkup("\\[\r\n" + formula + "\r\n\\]");
    expect(container.querySelectorAll(".katex-display")).toHaveLength(1);
    expect(container.querySelector(".katex-error")).toBeNull();
    expect(container.querySelector("annotation")?.textContent?.trim()).toBe(formula);
  });

  it("does not reinterpret delimiters within dollar math or raw HTML", () => {
    const container = mathMarkup(String.raw`$\text{\(literal\)}$

<pre>\[literal\]</pre>`);
    expect(container.querySelectorAll(".katex")).toHaveLength(1);
    expect(container.querySelector("annotation")?.textContent).toBe(String.raw`\text{\(literal\)}`);
    expect(container.textContent).toContain(String.raw`\[literal\]`);
  });

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

  it("displays known mentions by agent name", () => {
    const { container } = renderClient(
      <MarkdownRenderer text="Ask @codex and @unknown." mentionLabels={{ codex: "Codex" }} />
    );

    const mentions = Array.from(container.querySelectorAll<HTMLElement>("[data-mention]"));
    expect(mentions.map((mention) => mention.dataset.mention)).toEqual(["codex", "unknown"]);
    expect(mentions.map((mention) => mention.textContent)).toEqual(["@Codex", "@unknown"]);
    expect(mentions[0].getAttribute("title")).toBe("@codex");
    expect(container.textContent).toBe("Ask @Codex and @unknown.");
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

  it("renders unlabeled fenced code blocks without a typography pre wrapper", () => {
    const html = renderToStaticMarkup(
      <MarkdownRenderer text={"```\n| App A |  App B |\n|       |        |\n```"} />
    );

    expect(html).toContain("<code");
    expect(html).toContain("|       |        |");
    expect(html).not.toContain("<pre");
  });

  it("renders labeled fenced code blocks with syntax highlighted tokens", async () => {
    const { container } = renderClient(
      <div className="prose">
        <MarkdownRenderer text={"```ts\nconst x = 1;\n```"} />
      </div>
    );

    await waitFor(() => {
      expect(container.querySelector("[data-testid='code-block-shell']")).not.toBeNull();
    });

    const shell = container.querySelector<HTMLElement>("[data-testid='code-block-shell']");
    expect(shell?.className).toContain("not-prose");
    expect(container.querySelectorAll("[data-testid='code-block'] .token[style]").length)
      .toBeGreaterThan(0);
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

  it("opens relative markdown links to images and PDFs", () => {
    const opened: WorkspacePathTarget[] = [];
    const { container } = renderClient(
      <MarkdownRenderer
        text="[Diagram](../images/rpc.png), [Paper](./paper.pdf) and [Binary](../bin/tool)"
        workspacePath="/workspace/brpc"
        relativeLinkBasePath="docs/en/overview.md"
        onOpenWorkspacePath={(target) => opened.push(target)}
      />
    );

    const buttons = workspacePathButtons(container);
    expect(buttons.map((button) => button.textContent)).toEqual(["Diagram", "Paper"]);

    click(buttons[0]);
    click(buttons[1]);

    expect(opened).toEqual([
      { path: "docs/images/rpc.png", label: "docs/images/rpc.png" },
      { path: "docs/en/paper.pdf", label: "docs/en/paper.pdf" },
    ]);
  });

  it("loads relative workspace images through the file fetcher", async () => {
    const originalCreateObjectURL = URL.createObjectURL;
    const originalRevokeObjectURL = URL.revokeObjectURL;
    URL.createObjectURL = vi.fn(() => "blob:rpc");
    URL.revokeObjectURL = vi.fn();
    onTestFinished(() => {
      URL.createObjectURL = originalCreateObjectURL;
      URL.revokeObjectURL = originalRevokeObjectURL;
    });
    const fetchFile = vi.fn(async () => new Blob(["png"], { type: "image/png" }));
    const { container } = renderClient(
      <MarkdownRenderer
        text={"![rpc](../images/rpc.png)\n\n![remote](https://example.com/a.png)\n\n![up](../../../x.png)"}
        workspacePath="/workspace/brpc"
        relativeLinkBasePath="docs/en/overview.md"
        onFetchWorkspaceFile={fetchFile}
      />
    );

    const images = Array.from(container.querySelectorAll("img"));
    await waitFor(() => expect(images[0]?.getAttribute("src")).toBe("blob:rpc"));
    expect(fetchFile).toHaveBeenCalledTimes(1);
    expect(fetchFile).toHaveBeenCalledWith("docs/images/rpc.png");
    expect(images[0]?.getAttribute("alt")).toBe("rpc");
    expect(images[1]?.getAttribute("src")).toBe("https://example.com/a.png");
    expect(images[2]?.getAttribute("src")).toBe("../../../x.png");
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
