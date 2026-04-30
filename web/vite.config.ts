import path from "path";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

const cdnImports: Record<string, string> = {
  "@monaco-editor/react": "https://esm.sh/@monaco-editor/react@4.7.0?bundle&external=react",
  react: "https://esm.sh/react@19.2.5",
  "react/jsx-runtime": "https://esm.sh/react@19.2.5/jsx-runtime",
  "react-dom": "https://esm.sh/react-dom@19.2.5?bundle&external=react",
  "react-dom/client": "https://esm.sh/react-dom@19.2.5/client?bundle&external=react",
  "@tanstack/react-query": "https://esm.sh/@tanstack/react-query@5.100.5?bundle&external=react",
  "@radix-ui/react-avatar": "https://esm.sh/@radix-ui/react-avatar@1.1.11?bundle&external=react,react-dom",
  "@radix-ui/react-collapsible": "https://esm.sh/@radix-ui/react-collapsible@1.1.12?bundle&external=react,react-dom",
  "@radix-ui/react-context-menu": "https://esm.sh/@radix-ui/react-context-menu@2.2.16?bundle&external=react,react-dom",
  "@radix-ui/react-dialog": "https://esm.sh/@radix-ui/react-dialog@1.1.15?bundle&external=react,react-dom",
  "@radix-ui/react-label": "https://esm.sh/@radix-ui/react-label@2.1.8?bundle&external=react,react-dom",
  "@radix-ui/react-scroll-area": "https://esm.sh/@radix-ui/react-scroll-area@1.2.10?bundle&external=react,react-dom",
  "@radix-ui/react-select": "https://esm.sh/@radix-ui/react-select@2.2.6?bundle&external=react,react-dom",
  "@radix-ui/react-separator": "https://esm.sh/@radix-ui/react-separator@1.1.8?bundle&external=react,react-dom",
  "@radix-ui/react-slider": "https://esm.sh/@radix-ui/react-slider@1.3.6?bundle&external=react,react-dom",
  "@radix-ui/react-slot": "https://esm.sh/@radix-ui/react-slot@1.2.4?bundle&external=react,react-dom",
  "@radix-ui/react-switch": "https://esm.sh/@radix-ui/react-switch@1.2.6?bundle&external=react,react-dom",
  "@radix-ui/react-tabs": "https://esm.sh/@radix-ui/react-tabs@1.1.13?bundle&external=react,react-dom",
  "@radix-ui/react-toggle": "https://esm.sh/@radix-ui/react-toggle@1.1.10?bundle&external=react,react-dom",
  "@radix-ui/react-toggle-group": "https://esm.sh/@radix-ui/react-toggle-group@1.1.11?bundle&external=react,react-dom",
  "@radix-ui/react-tooltip": "https://esm.sh/@radix-ui/react-tooltip@1.2.8?bundle&external=react,react-dom",
  "class-variance-authority": "https://esm.sh/class-variance-authority@0.7.1?bundle",
  clsx: "https://esm.sh/clsx@2.1.1?bundle",
  katex: "https://esm.sh/katex@0.16.45?bundle",
  "lucide-react": "https://esm.sh/lucide-react@1.11.0?bundle&external=react",
  mermaid: "https://esm.sh/mermaid@11.14.0?bundle",
  "react-markdown": "https://esm.sh/react-markdown@10.1.0?bundle&external=react",
  "react-resizable-panels": "https://esm.sh/react-resizable-panels@2.1.9?bundle&external=react,react-dom",
  "react-syntax-highlighter/dist/esm/prism-light": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/prism-light?bundle&external=react",
  "react-syntax-highlighter/dist/esm/styles/prism/one-dark": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/styles/prism/one-dark?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/typescript": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/typescript?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/tsx": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/tsx?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/javascript": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/javascript?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/jsx": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/jsx?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/python": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/python?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/go": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/go?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/bash": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/bash?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/json": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/json?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/yaml": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/yaml?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/css": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/css?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/markup": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/markup?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/sql": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/sql?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/rust": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/rust?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/markdown": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/markdown?bundle",
  "react-syntax-highlighter/dist/esm/languages/prism/diff": "https://esm.sh/react-syntax-highlighter@16.1.1/dist/esm/languages/prism/diff?bundle",
  "rehype-katex": "https://esm.sh/rehype-katex@7.0.1?bundle",
  "remark-gfm": "https://esm.sh/remark-gfm@4.0.1?bundle",
  "remark-math": "https://esm.sh/remark-math@6.0.0?bundle",
  "tailwind-merge": "https://esm.sh/tailwind-merge@3.5.0?bundle"
};

const cdnImportPrefixes: Record<string, string> = {
  "react/": "https://esm.sh/react@19.2.5/",
  "react-dom/": "https://esm.sh/react-dom@19.2.5/"
};

function normalizeAssetBase(base: string | undefined) {
  const value = base?.trim();
  if (!value) return "/";
  return value.endsWith("/") ? value : `${value}/`;
}

const assetBase = normalizeAssetBase(process.env.AGENTX_WEB_ASSET_BASE);

function isCdnExternal(id: string) {
  return (
    Object.prototype.hasOwnProperty.call(cdnImports, id) ||
    Object.keys(cdnImportPrefixes).some((prefix) => id.startsWith(prefix))
  );
}

function cdnImportMapPlugin(): Plugin {
  return {
    name: "agentx-cdn-import-map",
    apply: "build",
    transformIndexHtml() {
      return [
        {
          tag: "script",
          attrs: { type: "importmap" },
          children: JSON.stringify({ imports: { ...cdnImportPrefixes, ...cdnImports } }, null, 2),
          injectTo: "head-prepend"
        }
      ];
    }
  };
}

export default defineConfig({
  base: assetBase,
  plugins: [react(), cdnImportMapPlugin()],
  build: {
    rolldownOptions: {
      external: isCdnExternal
    }
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src")
    }
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.AGENTX_API_TARGET ?? "http://127.0.0.1:8080",
        changeOrigin: true,
        ws: true
      }
    }
  },
  test: {
    exclude: ["e2e/**", "node_modules/**", "dist/**"]
  }
});
