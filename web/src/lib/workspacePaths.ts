export interface WorkspacePathTarget {
  path: string;
  lineNumber?: number;
  column?: number;
  label: string;
}

export type WorkspacePathTextSegment = string | WorkspacePathTarget;

const TRAILING_PUNCTUATION_RE = /[.,;)\]}]+$/;
const INVALID_PATH_CHARS_RE = /[\s<>"'|]/;
const GLOB_OR_QUERY_CHARS_RE = /[?#*{}[\]]/;
const RELATIVE_PATH_CANDIDATE_RE =
  /(^|[\s([{<"'`])((?:\/[^\s<>"'`]+|\.{1,2}\/[A-Za-z0-9._/-]+|[A-Za-z0-9._-]+(?:\/[A-Za-z0-9._-]+)+|[A-Za-z0-9._-]+\.[A-Za-z0-9][A-Za-z0-9._-]*)(?::[1-9][0-9]*(?::[1-9][0-9]*)?)?)/g;

const ROOT_FILE_NAMES = new Set([
  ".env",
  ".env.local",
  ".gitignore",
  "Dockerfile",
  "LICENSE",
  "Makefile",
]);

const ROOT_FILE_EXTENSIONS = new Set([
  "c",
  "cc",
  "cjs",
  "cpp",
  "cs",
  "css",
  "csv",
  "cts",
  "env",
  "erl",
  "ex",
  "fs",
  "gql",
  "go",
  "graphql",
  "h",
  "hcl",
  "hpp",
  "html",
  "java",
  "jl",
  "js",
  "json",
  "jsx",
  "kt",
  "lock",
  "md",
  "mjs",
  "mod",
  "mts",
  "php",
  "proto",
  "py",
  "r",
  "rb",
  "rs",
  "scss",
  "sh",
  "sql",
  "sum",
  "svelte",
  "swift",
  "tf",
  "toml",
  "ts",
  "tsx",
  "txt",
  "vue",
  "xml",
  "yaml",
  "yml",
  "zig",
]);

export function parseWorkspacePathTarget(
  value: string,
  workspacePath?: string
): WorkspacePathTarget | null {
  const workspaceRoot = normalizeWorkspaceRoot(workspacePath);
  if (!workspaceRoot) return null;

  const label = stripTrailingPunctuation(value.trim());
  if (!label) return null;
  if (isExternalReference(label)) return null;

  const parsed = splitLineColumn(label);
  if (!parsed) return null;

  const { pathPart, lineNumber, column } = parsed;
  if (INVALID_PATH_CHARS_RE.test(pathPart) || GLOB_OR_QUERY_CHARS_RE.test(pathPart)) {
    return null;
  }

  const relativePath = pathPart.startsWith("/")
    ? relativePathFromWorkspace(pathPart, workspaceRoot)
    : normalizeRelativeWorkspacePath(pathPart);
  if (!relativePath || !hasWorkspacePathShape(relativePath)) return null;

  return {
    path: relativePath,
    ...(lineNumber ? { lineNumber, column: column ?? 1 } : {}),
    label,
  };
}

export function splitWorkspacePathTargets(
  text: string,
  workspacePath?: string
): WorkspacePathTextSegment[] {
  const segments: WorkspacePathTextSegment[] = [];
  let consumed = 0;
  RELATIVE_PATH_CANDIDATE_RE.lastIndex = 0;

  for (const match of text.matchAll(RELATIVE_PATH_CANDIDATE_RE)) {
    const prefix = match[1] ?? "";
    const candidate = match[2] ?? "";
    const candidateStart = match.index + prefix.length;
    const candidateEnd = candidateStart + candidate.length;

    if (shouldSkipCandidateBoundary(text[candidateEnd])) {
      continue;
    }

    const target = parseWorkspacePathTarget(candidate, workspacePath);
    if (!target) continue;

    if (candidateStart > consumed) {
      segments.push(text.slice(consumed, candidateStart));
    }
    segments.push(target);
    // Advance past the matched label only so stripped punctuation stays as text.
    consumed = candidateStart + target.label.length;
  }

  if (consumed < text.length) {
    segments.push(text.slice(consumed));
  }

  return segments.length > 0 ? segments : [text];
}

function normalizeWorkspaceRoot(workspacePath?: string): string | null {
  const trimmed = workspacePath?.trim();
  if (!trimmed) return null;
  if (trimmed === "/") return "/";
  return trimmed.replace(/\/+$/, "");
}

function stripTrailingPunctuation(value: string): string {
  let next = value;
  while (TRAILING_PUNCTUATION_RE.test(next)) {
    next = next.replace(TRAILING_PUNCTUATION_RE, "");
  }
  return next;
}

function isExternalReference(value: string): boolean {
  return /^(?:https?:\/\/|mailto:|tel:)/i.test(value);
}

function splitLineColumn(value: string):
  | { pathPart: string; lineNumber?: number; column?: number }
  | null {
  const lineColumnMatch = /^(.*):([1-9][0-9]*):([1-9][0-9]*)$/.exec(value);
  if (lineColumnMatch) {
    const pathPart = lineColumnMatch[1];
    const lineNumber = Number.parseInt(lineColumnMatch[2], 10);
    const column = Number.parseInt(lineColumnMatch[3], 10);
    if (!pathPart) return null;
    if (!Number.isSafeInteger(lineNumber) || lineNumber < 1) return null;
    if (!Number.isSafeInteger(column) || column < 1) return null;
    return { pathPart, lineNumber, column };
  }

  const lineMatch = /^(.*):([1-9][0-9]*)$/.exec(value);
  if (!lineMatch) {
    return value.includes(":") ? null : { pathPart: value };
  }

  const pathPart = lineMatch[1];
  if (!pathPart) return null;
  const lineNumber = Number.parseInt(lineMatch[2], 10);
  if (!Number.isSafeInteger(lineNumber) || lineNumber < 1) return null;
  return { pathPart, lineNumber };
}

function relativePathFromWorkspace(absolutePath: string, workspaceRoot: string): string | null {
  const normalizedAbsolute = absolutePath.replace(/\/+$/, "");
  const rootPrefix = workspaceRoot === "/" ? "/" : `${workspaceRoot}/`;
  if (normalizedAbsolute === workspaceRoot) return null;
  if (!normalizedAbsolute.startsWith(rootPrefix)) return null;
  const relativePath = workspaceRoot === "/"
    ? normalizedAbsolute.slice(1)
    : normalizedAbsolute.slice(rootPrefix.length);
  return normalizeRelativeWorkspacePath(relativePath);
}

function normalizeRelativeWorkspacePath(path: string): string | null {
  let normalized = path;
  while (normalized.startsWith("./")) {
    normalized = normalized.slice(2);
  }
  if (!normalized || normalized.startsWith("/")) return null;
  const segments = normalized.split("/");
  if (
    segments.some((segment) => !segment || segment === "." || segment === "..")
  ) {
    return null;
  }
  return normalized;
}

function hasWorkspacePathShape(path: string): boolean {
  const segments = path.split("/");
  const basename = segments[segments.length - 1];
  if (!basename) return false;
  if (ROOT_FILE_NAMES.has(basename)) return true;
  const extension = basename.includes(".") ? basename.split(".").pop()?.toLowerCase() : undefined;
  return Boolean(extension && ROOT_FILE_EXTENSIONS.has(extension));
}

function shouldSkipCandidateBoundary(nextChar: string | undefined): boolean {
  return (
    nextChar === "?" ||
    nextChar === "#" ||
    nextChar === "*" ||
    nextChar === ":" ||
    nextChar === "="
  );
}
