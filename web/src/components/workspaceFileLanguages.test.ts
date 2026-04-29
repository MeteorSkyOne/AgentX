import { describe, expect, it } from "vitest";
import { isMarkdownFilePath, monacoLanguageForPath } from "./workspaceFileLanguages";

describe("monacoLanguageForPath", () => {
  it.each([
    ["src/App.tsx", "typescript"],
    ["src/index.mjs", "javascript"],
    ["package.json", "json"],
    ["docs/README.md", "markdown"],
    ["styles/theme.scss", "css"],
    ["public/index.html", "html"],
    ["cmd/agentx/main.go", "go"],
    ["scripts/seed.py", "python"],
    ["scripts/dev.sh", "shell"],
    [".github/workflows/test.yml", "yaml"],
    ["queries/report.sql", "sql"],
    ["crates/app/src/lib.rs", "rust"],
    ["native/main.c", "c"],
    ["native/include/app.h", "c"],
    ["native/main.cpp", "cpp"],
    ["native/lib.cc", "cpp"],
    ["native/include/app.hpp", "cpp"],
    ["src/Main.java", "java"],
    ["changes/fix.patch", "diff"],
    ["notes/plain.unknown", "plaintext"]
  ])("maps %s to %s", (path, language) => {
    expect(monacoLanguageForPath(path)).toBe(language);
  });

  it("is case-insensitive and defaults paths without extensions to plaintext", () => {
    expect(monacoLanguageForPath("README.MARKDOWN")).toBe("markdown");
    expect(monacoLanguageForPath("Makefile")).toBe("plaintext");
  });
});

describe("isMarkdownFilePath", () => {
  it("matches markdown file extensions", () => {
    expect(isMarkdownFilePath("docs/README.md")).toBe(true);
    expect(isMarkdownFilePath("docs/page.mdx")).toBe(true);
    expect(isMarkdownFilePath("README.MARKDOWN")).toBe(true);
    expect(isMarkdownFilePath("docs/readme.txt")).toBe(false);
  });
});
