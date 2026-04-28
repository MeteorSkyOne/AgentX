import { lazy, memo, Suspense, useMemo, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
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
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}

function renderMentions(text: string, keyPrefix: string): ReactNode[] {
  const parts = text.split(MENTION_RE);
  if (parts.length === 1) return [text];
  return parts.map((part, i) => {
    if (SINGLE_MENTION_RE.test(part)) {
      return (
        <span
          key={`${keyPrefix}-mention-${i}`}
          data-mention={part.slice(1)}
          className="rounded bg-primary/10 px-1 py-0.5 font-medium text-primary"
        >
          {part}
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
    return renderMentions(text, keyPrefix);
  }

  const onOpenWorkspacePath = options.onOpenWorkspacePath;
  return splitWorkspacePathTargets(text, options.workspacePath).flatMap((segment, i) => {
    if (typeof segment === "string") {
      return renderMentions(segment, `${keyPrefix}-${i}`);
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
      const isFencedCode = /\blanguage-/.test(props.className ?? "");
      const target = isFencedCode
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
          <CodeBlock {...props} />
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

      const target = href
        ? parseWorkspacePathTarget(href, options.workspacePath)
        : null;
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

interface Props {
  text: string;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}

export const MarkdownRenderer = memo(function MarkdownRenderer({
  text,
  workspacePath,
  onOpenWorkspacePath,
}: Props) {
  const components = useMemo(
    () => createComponents({ workspacePath, onOpenWorkspacePath }),
    [workspacePath, onOpenWorkspacePath]
  );

  return (
    <Markdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeKatex]}
      components={components}
    >
      {text}
    </Markdown>
  );
});
