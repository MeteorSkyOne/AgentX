const extensionLanguages: Record<string, string> = {
  bash: "shell",
  c: "c",
  cc: "cpp",
  cjs: "javascript",
  cpp: "cpp",
  css: "css",
  cts: "typescript",
  cxx: "cpp",
  diff: "diff",
  fish: "shell",
  go: "go",
  h: "c",
  hh: "cpp",
  hpp: "cpp",
  htm: "html",
  html: "html",
  hxx: "cpp",
  java: "java",
  js: "javascript",
  json: "json",
  jsonc: "json",
  jsx: "javascript",
  ksh: "shell",
  markdown: "markdown",
  md: "markdown",
  mdx: "markdown",
  mjs: "javascript",
  mts: "typescript",
  patch: "diff",
  py: "python",
  pyw: "python",
  rs: "rust",
  scss: "css",
  sh: "shell",
  sql: "sql",
  ts: "typescript",
  tsx: "typescript",
  yaml: "yaml",
  yml: "yaml",
  zsh: "shell"
};

export function monacoLanguageForPath(path: string): string {
  const fileName = path.trim().split(/[\\/]/).pop()?.toLowerCase() ?? "";
  const extension = fileName.includes(".") ? fileName.split(".").pop() ?? "" : "";
  return extensionLanguages[extension] ?? "plaintext";
}

export function isMarkdownFilePath(path: string): boolean {
  return monacoLanguageForPath(path) === "markdown";
}
