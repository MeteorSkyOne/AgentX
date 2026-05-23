const maxTerminalClipboardBytes = 1024 * 1024;

export function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunkSize = 0x8000;
  for (let index = 0; index < bytes.length; index += chunkSize) {
    const chunk = bytes.subarray(index, index + chunkSize);
    binary += String.fromCharCode(...chunk);
  }
  return btoa(binary);
}

export function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

export function binaryStringToBytes(value: string): Uint8Array {
  const bytes = new Uint8Array(value.length);
  for (let index = 0; index < value.length; index += 1) {
    bytes[index] = value.charCodeAt(index) & 0xff;
  }
  return bytes;
}

export function terminalOsc52ClipboardText(data: string): string | null {
  const separator = data.indexOf(";");
  if (separator < 0) return null;
  const payload = data.slice(separator + 1).replace(/\s/g, "");
  if (payload === "" || payload === "?") return null;
  try {
    const bytes = base64ToBytes(payload);
    if (bytes.byteLength > maxTerminalClipboardBytes) return null;
    return new TextDecoder().decode(bytes);
  } catch {
    return null;
  }
}

export async function copyTerminalText(text: string): Promise<void> {
  if (!text) return;
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through to execCommand for browsers that gate async clipboard writes.
    }
  }
  copyTerminalTextWithExecCommand(text);
}

export function copyTerminalTextWithExecCommand(text: string): void {
  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
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
      throw new Error("Copy terminal selection failed");
    }
  } finally {
    textarea.remove();
    activeElement?.focus({ preventScroll: true });
  }
}
