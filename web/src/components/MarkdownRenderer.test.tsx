import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { MarkdownRenderer } from "./MarkdownRenderer";

describe("MarkdownRenderer", () => {
  it("renders inline and block math with KaTeX markup", () => {
    const html = renderToStaticMarkup(
      <MarkdownRenderer text={"Inline $E = mc^2$.\n\n$$\na^2 + b^2 = c^2\n$$"} />
    );

    expect(html).toContain("katex");
    expect(html).toContain("katex-display");
    expect(html).toContain("aria-hidden=\"true\"");
  });
});
