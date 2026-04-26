import type { Message } from "../api/types";

export type BrowserNotificationPermission = NotificationPermission | "unsupported";

interface NotificationRuntime {
  Notification?: typeof Notification;
  document?: Pick<Document, "hidden" | "hasFocus">;
  window?: Pick<Window, "focus">;
}

export function browserNotificationPermission(
  runtime: NotificationRuntime = {}
): BrowserNotificationPermission {
  const api = notificationAPI(runtime);
  return api ? api.permission : "unsupported";
}

export async function requestBrowserNotificationPermission(
  runtime: NotificationRuntime = {}
): Promise<BrowserNotificationPermission> {
  const api = notificationAPI(runtime);
  if (!api) {
    return "unsupported";
  }
  if (api.permission === "granted" || api.permission === "denied") {
    return api.permission;
  }
  return api.requestPermission();
}

export function pageIsInactive(runtime: NotificationRuntime = {}): boolean {
  const doc = runtime.document ?? globalDocument();
  if (!doc) {
    return false;
  }
  return Boolean(doc.hidden) || (typeof doc.hasFocus === "function" && !doc.hasFocus());
}

export function shouldShowBrowserNotification(
  message: Message,
  runtime: NotificationRuntime = {}
): boolean {
  return (
    message.sender_type === "bot" &&
    browserNotificationPermission(runtime) === "granted" &&
    pageIsInactive(runtime)
  );
}

export function showAgentMessageNotification(
  message: Message,
  runtime: NotificationRuntime = {}
): boolean {
  const api = notificationAPI(runtime);
  if (!api || !shouldShowBrowserNotification(message, runtime)) {
    return false;
  }
  const notification = new api("Agent reply", {
    body: notificationBody(message.body),
    tag: `agentx:${message.id}`
  });
  notification.onclick = () => {
    const win = runtime.window ?? globalWindow();
    win?.focus();
  };
  return true;
}

function notificationAPI(runtime: NotificationRuntime): typeof Notification | undefined {
  if (runtime.Notification) {
    return runtime.Notification;
  }
  if (typeof Notification === "undefined") {
    return undefined;
  }
  return Notification;
}

function globalDocument(): Pick<Document, "hidden" | "hasFocus"> | undefined {
  if (typeof document === "undefined") {
    return undefined;
  }
  return document;
}

function globalWindow(): Pick<Window, "focus"> | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  return window;
}

function notificationBody(value: string): string {
  const body = value.trim().replace(/\s+/g, " ");
  if (!body) {
    return "New message from an agent";
  }
  if (body.length <= 140) {
    return body;
  }
  return `${body.slice(0, 137)}...`;
}
