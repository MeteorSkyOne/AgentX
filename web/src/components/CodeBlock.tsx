import { useCallback, useEffect, useState, type ReactNode } from "react";
import SyntaxHighlighter from "react-syntax-highlighter/dist/esm/prism-light";
import oneDark from "react-syntax-highlighter/dist/esm/styles/prism/one-dark";
import oneLight from "react-syntax-highlighter/dist/esm/styles/prism/one-light";
import { Copy, Check } from "lucide-react";
import { D2Diagram } from "./D2Diagram";
import { MermaidDiagram } from "./MermaidDiagram";

import typescript from "react-syntax-highlighter/dist/esm/languages/prism/typescript";
import tsx from "react-syntax-highlighter/dist/esm/languages/prism/tsx";
import javascript from "react-syntax-highlighter/dist/esm/languages/prism/javascript";
import jsx from "react-syntax-highlighter/dist/esm/languages/prism/jsx";
import python from "react-syntax-highlighter/dist/esm/languages/prism/python";
import go from "react-syntax-highlighter/dist/esm/languages/prism/go";
import bash from "react-syntax-highlighter/dist/esm/languages/prism/bash";
import json from "react-syntax-highlighter/dist/esm/languages/prism/json";
import yaml from "react-syntax-highlighter/dist/esm/languages/prism/yaml";
import css from "react-syntax-highlighter/dist/esm/languages/prism/css";
import markup from "react-syntax-highlighter/dist/esm/languages/prism/markup";
import sql from "react-syntax-highlighter/dist/esm/languages/prism/sql";
import rust from "react-syntax-highlighter/dist/esm/languages/prism/rust";
import markdown from "react-syntax-highlighter/dist/esm/languages/prism/markdown";
import diff from "react-syntax-highlighter/dist/esm/languages/prism/diff";

SyntaxHighlighter.registerLanguage("typescript", typescript);
SyntaxHighlighter.registerLanguage("ts", typescript);
SyntaxHighlighter.registerLanguage("tsx", tsx);
SyntaxHighlighter.registerLanguage("javascript", javascript);
SyntaxHighlighter.registerLanguage("js", javascript);
SyntaxHighlighter.registerLanguage("jsx", jsx);
SyntaxHighlighter.registerLanguage("python", python);
SyntaxHighlighter.registerLanguage("py", python);
SyntaxHighlighter.registerLanguage("go", go);
SyntaxHighlighter.registerLanguage("bash", bash);
SyntaxHighlighter.registerLanguage("sh", bash);
SyntaxHighlighter.registerLanguage("shell", bash);
SyntaxHighlighter.registerLanguage("json", json);
SyntaxHighlighter.registerLanguage("yaml", yaml);
SyntaxHighlighter.registerLanguage("yml", yaml);
SyntaxHighlighter.registerLanguage("css", css);
SyntaxHighlighter.registerLanguage("html", markup);
SyntaxHighlighter.registerLanguage("xml", markup);
SyntaxHighlighter.registerLanguage("markup", markup);
SyntaxHighlighter.registerLanguage("sql", sql);
SyntaxHighlighter.registerLanguage("rust", rust);
SyntaxHighlighter.registerLanguage("rs", rust);
SyntaxHighlighter.registerLanguage("markdown", markdown);
SyntaxHighlighter.registerLanguage("md", markdown);
SyntaxHighlighter.registerLanguage("diff", diff);

type CodeTheme = "light" | "dark";

function createCodeStyle(style: Record<string, React.CSSProperties>): Record<string, React.CSSProperties> {
  return {
    ...style,
    'pre[class*="language-"]': {
      ...(style['pre[class*="language-"]'] as React.CSSProperties),
      background: "transparent",
      margin: 0,
      padding: "0.75rem",
      fontSize: "0.875rem",
    },
    'code[class*="language-"]': {
      ...(style['code[class*="language-"]'] as React.CSSProperties),
      background: "transparent",
    },
  };
}

const codeStyles: Record<CodeTheme, Record<string, React.CSSProperties>> = {
  dark: createCodeStyle(oneDark),
  light: createCodeStyle(oneLight),
};

function documentTheme(): CodeTheme {
  if (typeof document === "undefined") return "dark";
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

function useDocumentTheme(): CodeTheme {
  const [theme, setTheme] = useState<CodeTheme>(() => documentTheme());

  useEffect(() => {
    if (typeof document === "undefined") return;

    const root = document.documentElement;
    const updateTheme = () => setTheme(documentTheme());
    updateTheme();

    const observer = new MutationObserver(updateTheme);
    observer.observe(root, { attributes: true, attributeFilter: ["class"] });

    return () => observer.disconnect();
  }, []);

  return theme;
}

export function CodeBlock({
  block = false,
  className,
  children,
}: {
  block?: boolean;
  className?: string;
  children?: ReactNode;
}) {
  const match = /language-([\w-]+)/.exec(className || "");

  if (!match && !block) {
    return <code className={className}>{children}</code>;
  }

  const language = match?.[1].toLowerCase() ?? "";
  const code = codeBlockText(children).replace(/\n$/, "");

  if (language === "mermaid" || language === "mmd") {
    return <MermaidDiagram source={code} />;
  }
  if (language === "d2") {
    return <D2Diagram source={code} />;
  }

  return <FencedCodeBlock language={language} code={code} />;
}

function codeBlockText(children: ReactNode): string {
  if (Array.isArray(children)) {
    return children.map(codeBlockText).join("");
  }
  if (children === null || children === undefined) {
    return "";
  }
  return String(children);
}

function FencedCodeBlock({
  language,
  code,
}: {
  language: string;
  code: string;
}) {
  const [copied, setCopied] = useState(false);
  const theme = useDocumentTheme();
  const codeStyle = codeStyles[theme];

  const handleCopy = useCallback(async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [code]);

  return (
    <div
      className="group/code relative my-2 min-w-0 w-full max-w-full overflow-hidden rounded-md border border-border bg-muted/60 dark:bg-sidebar"
      data-testid="code-block-shell"
    >
      <div className="flex items-center justify-between px-3 pt-2">
        {language && (
          <span className="text-xs text-muted-foreground">{language}</span>
        )}
        <button
          onClick={handleCopy}
          className="rounded p-1 text-muted-foreground opacity-100 transition-opacity hover:text-foreground md:opacity-0 md:group-hover/code:opacity-100"
          aria-label="Copy code"
          title="Copy code"
        >
          {copied ? (
            <Check className="h-4 w-4" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </button>
      </div>
      <div className="min-w-0 w-full max-w-full overflow-x-auto" data-testid="code-block">
        <SyntaxHighlighter
          language={language || undefined}
          style={codeStyle}
          PreTag="div"
          codeTagProps={{
            style: {
              background: "transparent",
              borderRadius: 0,
              fontSize: "inherit",
              fontWeight: "inherit",
              overflowWrap: "normal",
              padding: 0,
              whiteSpace: "pre",
              wordBreak: "normal",
            },
          }}
          customStyle={{
            background: "transparent",
            margin: 0,
            minWidth: "max-content",
            overflowWrap: "normal",
            whiteSpace: "pre",
            wordBreak: "normal",
          }}
          wrapLongLines={false}
        >
          {code}
        </SyntaxHighlighter>
      </div>
    </div>
  );
}
