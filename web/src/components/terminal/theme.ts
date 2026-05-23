import type { ThemeMode } from "@/theme";

export function terminalTheme(theme: ThemeMode) {
  if (theme === "light") {
    return {
      background: "#f8fafc",
      foreground: "#0f172a",
      cursor: "#0f172a",
      selectionBackground: "#cbd5e1",
      black: "#0f172a",
      red: "#dc2626",
      green: "#15803d",
      yellow: "#a16207",
      blue: "#2563eb",
      magenta: "#9333ea",
      cyan: "#0891b2",
      white: "#e2e8f0",
    };
  }
  return {
    background: "#111315",
    foreground: "#e5e7eb",
    cursor: "#f8fafc",
    selectionBackground: "#374151",
    black: "#111827",
    red: "#f87171",
    green: "#4ade80",
    yellow: "#facc15",
    blue: "#60a5fa",
    magenta: "#c084fc",
    cyan: "#22d3ee",
    white: "#f9fafb",
  };
}
