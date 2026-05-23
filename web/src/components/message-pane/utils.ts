const AUTO_SCROLL_BOTTOM_THRESHOLD_PX = 80;

export function isNearViewportBottom(viewport: HTMLElement): boolean {
  const distanceFromBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight;
  return distanceFromBottom <= AUTO_SCROLL_BOTTOM_THRESHOLD_PX;
}

export function cssEscape(value: string): string {
  if (typeof CSS !== "undefined" && CSS.escape) {
    return CSS.escape(value);
  }
  return value.replace(/"/g, '\\"');
}

export async function copyTextToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall back for browsers or test environments that block async clipboard writes.
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "-9999px";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();

  try {
    if (!document.execCommand("copy")) {
      throw new Error("Copy message failed");
    }
  } finally {
    textarea.remove();
  }
}
