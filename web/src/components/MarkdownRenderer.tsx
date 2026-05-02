import {
  lazy,
  memo,
  Suspense,
  useMemo,
  type ReactNode,
} from "react";
import Markdown, { defaultUrlTransform, type Components, type UrlTransform } from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import {
  parseWorkspacePathTarget,
  splitWorkspacePathTargets,
  type WorkspacePathTarget,
} from "@/lib/workspacePaths";

const CodeBlock = lazy(() =>
  import("./CodeBlock").then((module) => ({ default: module.CodeBlock }))
);

const MENTION_RE = /(@[A-Za-z0-9][A-Za-z0-9_-]*)/g;
const SINGLE_MENTION_RE = /^@[A-Za-z0-9][A-Za-z0-9_-]*$/;

interface RendererOptions {
  workspacePath?: string;
  relativeLinkBasePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
  mentionLabels?: MentionLabels;
}

export type MentionLabels = Record<string, string>;

function mentionDisplayValue(handle: string, mentionLabels?: MentionLabels): string {
  const label = mentionLabels?.[handle.toLowerCase()]?.trim();
  return label ? `@${label}` : `@${handle}`;
}

function renderMentions(
  text: string,
  keyPrefix: string,
  mentionLabels?: MentionLabels
): ReactNode[] {
  const parts = text.split(MENTION_RE);
  if (parts.length === 1) return [text];
  return parts.map((part, i) => {
    if (SINGLE_MENTION_RE.test(part)) {
      const handle = part.slice(1);
      const display = mentionDisplayValue(handle, mentionLabels);
      return (
        <span
          key={`${keyPrefix}-mention-${i}`}
          data-mention={handle}
          className="rounded bg-primary/10 px-1 py-0.5 font-medium text-primary"
          title={display === part ? undefined : part}
        >
          {display}
        </span>
      );
    }
    return part;
  });
}

function renderInlineText(
  text: string,
  options: RendererOptions,
  keyPrefix: string
): ReactNode[] {
  if (!options.workspacePath || !options.onOpenWorkspacePath) {
    return renderMentions(text, keyPrefix, options.mentionLabels);
  }

  const onOpenWorkspacePath = options.onOpenWorkspacePath;
  return splitWorkspacePathTargets(text, options.workspacePath).flatMap((segment, i) => {
    if (typeof segment === "string") {
      return renderMentions(segment, `${keyPrefix}-${i}`, options.mentionLabels);
    }
    return (
      <WorkspacePathButton
        key={`${keyPrefix}-path-${i}`}
        target={segment}
        onOpen={onOpenWorkspacePath}
      />
    );
  });
}

function renderInlineChildren(
  children: ReactNode,
  options: RendererOptions,
  keyPrefix: string
): ReactNode {
  if (typeof children === "string") {
    return renderInlineText(children, options, keyPrefix);
  }
  if (Array.isArray(children)) {
    return children.map((child, i) => (
      typeof child === "string"
        ? renderInlineText(child, options, `${keyPrefix}-${i}`)
        : child
    ));
  }
  return children;
}

function WorkspacePathButton({
  target,
  onOpen,
  className,
  children,
}: {
  target: WorkspacePathTarget;
  onOpen: (target: WorkspacePathTarget) => void;
  className?: string;
  children?: ReactNode;
}) {
  const location = target.lineNumber
    ? `:${target.lineNumber}${target.column ? `:${target.column}` : ""}`
    : "";
  return (
    <button
      type="button"
      className={[
        "inline max-w-full cursor-pointer rounded-sm border-0 bg-transparent p-0 text-left font-[inherit] text-primary underline decoration-primary/40 underline-offset-2 [overflow-wrap:anywhere] hover:text-primary/80 hover:decoration-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
        className,
      ].filter(Boolean).join(" ")}
      data-workspace-path={target.path}
      data-workspace-line={target.lineNumber}
      data-workspace-column={target.column}
      title={`Open ${target.path}${location}`}
      aria-label={`Open ${target.path}${location}`}
      onClick={() => {
        onOpen(target);
      }}
    >
      {children ?? target.label}
    </button>
  );
}

function createComponents(options: RendererOptions): Components {
  return {
    pre: ({ children }) => <>{children}</>,
    code: ({ node: _node, ...props }) => {
      const isBlockCode = isMarkdownCodeBlock(props.className, props.children);
      const target = isBlockCode
        ? null
        : inlineCodePathTarget(props.children, options.workspacePath);
      if (target && options.onOpenWorkspacePath) {
        return (
          <WorkspacePathButton
            target={target}
            onOpen={options.onOpenWorkspacePath}
            className="font-mono text-[0.875em]"
          >
            <code className={props.className}>{props.children}</code>
          </WorkspacePathButton>
        );
      }

      return (
        <Suspense fallback={<code className={props.className}>{props.children}</code>}>
          <CodeBlock block={isBlockCode} {...props} />
        </Suspense>
      );
    },
    a: ({ href, children, node: _node, ...props }) => {
      const isRealUrl = href && /^https?:\/\/|^mailto:|^tel:/i.test(href);
      if (isRealUrl) {
        return (
          <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
            {children}
          </a>
        );
      }

      const target = href ? markdownLinkTarget(href, options) : null;
      if (target && options.onOpenWorkspacePath) {
        return (
          <WorkspacePathButton
            target={target}
            onOpen={options.onOpenWorkspacePath}
          >
            {children}
          </WorkspacePathButton>
        );
      }

      return <>{children}</>;
    },
    p: ({ children, node: _node, ...props }) => (
      <p {...props}>{renderInlineChildren(children, options, "p")}</p>
    ),
    li: ({ children, node: _node, ...props }) => (
      <li {...props}>{renderInlineChildren(children, options, "li")}</li>
    ),
    td: ({ children, node: _node, ...props }) => (
      <td {...props}>{renderInlineChildren(children, options, "td")}</td>
    ),
    table: ({ children, className, node: _node, ...props }) => (
      <div className="min-w-0 w-full max-w-full overflow-x-auto">
        <table
          {...props}
          className={["table-fixed w-full max-w-full", className].filter(Boolean).join(" ")}
        >
          {children}
        </table>
      </div>
    ),
  };
}

function isMarkdownCodeBlock(className: string | undefined, children: ReactNode): boolean {
  if (/\blanguage-/.test(className ?? "")) return true;
  const text = singleTextChild(children);
  return Boolean(text && text.includes("\n"));
}

function inlineCodePathTarget(
  children: ReactNode,
  workspacePath?: string
): WorkspacePathTarget | null {
  if (!workspacePath) return null;
  const text = singleTextChild(children);
  if (!text || text.trim() !== text) return null;
  return parseWorkspacePathTarget(text, workspacePath);
}

function singleTextChild(children: ReactNode): string | null {
  if (typeof children === "string") return children;
  if (Array.isArray(children) && children.length === 1 && typeof children[0] === "string") {
    return children[0];
  }
  return null;
}

function markdownLinkTarget(
  href: string,
  options: RendererOptions
): WorkspacePathTarget | null {
  const targetHref = options.relativeLinkBasePath
    ? resolveMarkdownRelativeHref(href, options.relativeLinkBasePath)
    : href;
  return parseWorkspacePathTarget(targetHref, options.workspacePath);
}

function resolveMarkdownRelativeHref(href: string, basePath: string): string {
  const trimmed = href.trim();

  const parsed = splitMarkdownHrefLocation(trimmed);
  if (!parsed) return href;
  if (!isRelativeMarkdownPath(parsed.path)) return href;

  const baseDirectory = parentWorkspaceDirectoryPath(basePath);
  const resolvedPath = normalizeJoinedWorkspacePath(baseDirectory, parsed.path);
  return resolvedPath ? `${resolvedPath}${parsed.location}` : href;
}

function isRelativeMarkdownPath(path: string): boolean {
  return Boolean(path) && !/^(?:[a-z][a-z0-9+.-]*:|#|\/)/i.test(path);
}

function splitMarkdownHrefLocation(
  href: string
): { path: string; location: string } | null {
  const match = /^(.*?)(:[1-9][0-9]*(?::[1-9][0-9]*)?)?$/.exec(href);
  if (!match) return null;
  const path = match[1] ?? "";
  if (!path) return null;
  return { path, location: match[2] ?? "" };
}

function parentWorkspaceDirectoryPath(path: string): string {
  const normalized = path.trim().replaceAll("\\", "/").replace(/^\/+|\/+$/g, "");
  const slashIndex = normalized.lastIndexOf("/");
  return slashIndex === -1 ? "" : normalized.slice(0, slashIndex);
}

function normalizeJoinedWorkspacePath(basePath: string, hrefPath: string): string | null {
  const segments: string[] = [];
  const combined = basePath ? `${basePath}/${hrefPath}` : hrefPath;

  for (const segment of combined.replaceAll("\\", "/").split("/")) {
    if (!segment || segment === ".") continue;
    if (segment === "..") {
      if (segments.length === 0) return null;
      segments.pop();
      continue;
    }
    segments.push(segment);
  }

  return segments.length > 0 ? segments.join("/") : null;
}

const markdownUrlTransform: UrlTransform = (url, key, node) => {
  if (key === "href" && node.tagName === "a") {
    return url;
  }
  return defaultUrlTransform(url);
};

interface Props {
  text: string;
  workspacePath?: string;
  relativeLinkBasePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
  mentionLabels?: MentionLabels;
}

export const MarkdownRenderer = memo(function MarkdownRenderer({
  text,
  workspacePath,
  relativeLinkBasePath,
  onOpenWorkspacePath,
  mentionLabels,
}: Props) {
  const components = useMemo(
    () => createComponents({ workspacePath, relativeLinkBasePath, onOpenWorkspacePath, mentionLabels }),
    [workspacePath, relativeLinkBasePath, onOpenWorkspacePath, mentionLabels]
  );

  return (
    <Markdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeKatex]}
      components={components}
      urlTransform={markdownUrlTransform}
    >
      {text}
    </Markdown>
  );
});
