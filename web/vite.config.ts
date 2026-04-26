import path from "path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
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
