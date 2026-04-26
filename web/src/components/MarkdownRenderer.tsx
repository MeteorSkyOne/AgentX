import { memo, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { CodeBlock } from "./CodeBlock";

const MENTION_RE = /(@[A-Za-z0-9][A-Za-z0-9_-]*)/g;

function renderMentions(text: string): ReactNode[] {
  const parts = text.split(MENTION_RE);
  if (parts.length === 1) return [text];
  return parts.map((part, i) => {
    if (MENTION_RE.test(part)) {
      MENTION_RE.lastIndex = 0;
      return (
        <span
          key={i}
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

const components: Components = {
  pre: ({ children }) => <>{children}</>,
  code: CodeBlock as Components["code"],
  a: ({ href, children, node: _node, ...props }) => (
    <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
      {children}
    </a>
  ),
  p: ({ children, node: _node, ...props }) => {
    if (typeof children === "string") {
      return <p {...props}>{renderMentions(children)}</p>;
    }
    return <p {...props}>{children}</p>;
  },
  li: ({ children, node: _node, ...props }) => {
    if (typeof children === "string") {
      return <li {...props}>{renderMentions(children)}</li>;
    }
    return <li {...props}>{children}</li>;
  },
  td: ({ children, node: _node, ...props }) => {
    if (typeof children === "string") {
      return <td {...props}>{renderMentions(children)}</td>;
    }
    return <td {...props}>{children}</td>;
  },
  table: ({ children, node: _node, ...props }) => (
    <div className="overflow-x-auto">
      <table {...props}>{children}</table>
    </div>
  ),
};

interface Props {
  text: string;
}

export const MarkdownRenderer = memo(function MarkdownRenderer({
  text,
}: Props) {
  return (
    <Markdown remarkPlugins={[remarkGfm]} components={components}>
      {text}
    </Markdown>
  );
});
