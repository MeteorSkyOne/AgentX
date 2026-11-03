import type { CompileContext, Extension as FromMarkdownExtension } from "mdast-util-from-markdown";
import type { Code, Extension, State, Token, Tokenizer } from "micromark-util-types";
import type { Plugin } from "unified";

declare module "micromark-util-types" {
  interface TokenTypeMap {
    latexMathInline: "latexMathInline";
    latexMathDisplay: "latexMathDisplay";
    latexMathData: "latexMathData";
  }
}

// Micromark represents LF, CR and CRLF as negative character codes.
function isLineEnding(code: Code) {
  return code === -5 || code === -4 || code === -3;
}

// Parse before Markdown's character escapes, so underscores and backslashes
// inside formulas stay intact. Code, HTML and existing math keep their parsers.
const tokenize: Tokenizer = function (effects, ok, nok) {
  let closing: number;
  let type: "latexMathInline" | "latexMathDisplay";

  return start;

  function start(code: Code) {
    // The token type is updated after reading the opening delimiter.
    effects.enter("latexMathInline");
    effects.enter("latexMathData");
    effects.consume(code);
    return opener;
  }

  function opener(code: Code) {
    if (code !== 40 && code !== 91) return nok(code);
    closing = code === 40 ? 41 : 93;
    type = code === 40 ? "latexMathInline" : "latexMathDisplay";
    effects.consume(code);
    return body;
  }

  function body(code: Code): State | undefined {
    if (code === null) return nok(code);
    // Micromark needs explicit line-ending events to join multiline chunks.
    if (isLineEnding(code)) {
      effects.exit("latexMathData");
      effects.enter("lineEnding");
      effects.consume(code);
      effects.exit("lineEnding");
      return afterLineEnding;
    }
    effects.consume(code);
    return code === 92 ? escaped : body;
  }

  function afterLineEnding(code: Code): State | undefined {
    effects.enter("latexMathData");
    return body(code);
  }

  function escaped(code: Code): State | undefined {
    if (code === null) return nok(code);
    if (isLineEnding(code)) return body(code);
    effects.consume(code);
    if (code !== closing) return body;
    effects.exit("latexMathData");
    const token = effects.exit("latexMathInline");
    token.type = type;
    return ok;
  }
};

const syntax: Extension = { text: { 92: { name: "latexMath", tokenize } } };

const fromMarkdown: FromMarkdownExtension = {
  enter: {
    latexMathInline(token) {
      this.enter({ type: "inlineMath", value: "" }, token);
      this.buffer();
    },
    latexMathDisplay(token) {
      this.enter({ type: "inlineMath", value: "" }, token);
      this.buffer();
    },
  },
  exit: {
    latexMathInline: finishMath,
    latexMathDisplay: finishMath,
  },
};

function finishMath(
  this: CompileContext,
  token: Token,
) {
  this.resume();
  const node = this.stack[this.stack.length - 1];
  if (node.type !== "inlineMath") return;
  node.value = this.sliceSerialize(token).slice(2, -2);
  node.data = {
    hName: "code",
    hProperties: {
      className: ["language-math", token.type === "latexMathDisplay" ? "math-display" : "math-inline"],
    },
    hChildren: [{ type: "text", value: node.value }],
  };
  this.exit(token);
}

export const remarkLatexMath: Plugin = function () {
  const data = this.data() as {
    micromarkExtensions?: Extension[];
    fromMarkdownExtensions?: FromMarkdownExtension[];
  };
  (data.micromarkExtensions ??= []).push(syntax);
  (data.fromMarkdownExtensions ??= []).push(fromMarkdown);
};
