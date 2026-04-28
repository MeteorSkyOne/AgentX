import { describe, expect, it, vi } from "vitest";
import type { Message } from "../api/types";
import {
  pageIsInactive,
  requestBrowserNotificationPermission,
  shouldShowBrowserNotification,
  showAgentMessageNotification
} from "./browser";

describe("browser notifications", () => {
  it("only allows bot message notifications when the page is inactive and permission is granted", () => {
    const runtime = {
      Notification: notificationClass("granted"),
      document: { hidden: false, hasFocus: () => false }
    };

    expect(shouldShowBrowserNotification(message("bot"), runtime)).toBe(true);
    expect(shouldShowBrowserNotification(message("user"), runtime)).toBe(false);
    expect(shouldShowBrowserNotification(teamMessage("discussion"), runtime)).toBe(false);
    expect(shouldShowBrowserNotification(teamMessage("leader"), runtime)).toBe(false);
    expect(shouldShowBrowserNotification(teamMessage("summary"), runtime)).toBe(true);
    expect(
      shouldShowBrowserNotification(message("bot"), {
        Notification: notificationClass("granted"),
        document: { hidden: false, hasFocus: () => true }
      })
    ).toBe(false);
    expect(
      shouldShowBrowserNotification(message("bot"), {
        Notification: notificationClass("denied"),
        document: { hidden: true, hasFocus: () => true }
      })
    ).toBe(false);
  });

  it("requests permission through the Notification API", async () => {
    const requestPermission = vi.fn(async () => "granted" as NotificationPermission);
    const Notification = notificationClass("default", requestPermission);

    await expect(requestBrowserNotificationPermission({ Notification })).resolves.toBe("granted");
    expect(requestPermission).toHaveBeenCalledTimes(1);
  });

  it("focuses the window when a notification is clicked", () => {
    const notifications: MockNotification[] = [];
    const focus = vi.fn();
    const Notification = notificationClass("granted", undefined, notifications);

    expect(
      showAgentMessageNotification(message("bot"), {
        Notification,
        document: { hidden: true, hasFocus: () => false },
        window: { focus }
      })
    ).toBe(true);

    notifications[0].onclick?.(new Event("click"));
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it("treats hidden and unfocused documents as inactive", () => {
    expect(pageIsInactive({ document: { hidden: true, hasFocus: () => true } })).toBe(true);
    expect(pageIsInactive({ document: { hidden: false, hasFocus: () => false } })).toBe(true);
    expect(pageIsInactive({ document: { hidden: false, hasFocus: () => true } })).toBe(false);
  });
});

class MockNotification {
  static permission: NotificationPermission = "default";
  static requestPermission: () => Promise<NotificationPermission> = async () => "default";

  onclick: ((event: Event) => void) | null = null;

  constructor(
    public title: string,
    public options?: NotificationOptions
  ) {}
}

function notificationClass(
  permission: NotificationPermission,
  requestPermission?: () => Promise<NotificationPermission>,
  instances: MockNotification[] = []
): typeof Notification {
  const permissionValue = permission;
  const requestPermissionFn = requestPermission ?? vi.fn(async () => permissionValue);
  class TestNotification extends MockNotification {
    static override permission: NotificationPermission = permissionValue;
    static override requestPermission = requestPermissionFn;

    constructor(title: string, options?: NotificationOptions) {
      super(title, options);
      instances.push(this);
    }
  }
  return TestNotification as unknown as typeof Notification;
}

function message(senderType: Message["sender_type"]): Message {
  return {
    id: `msg_${senderType}`,
    organization_id: "org_1",
    conversation_type: "channel",
    conversation_id: "chn_1",
    sender_type: senderType,
    sender_id: senderType,
    kind: "text",
    body: "Reply text",
    created_at: "2026-04-25T10:00:00Z"
  };
}

function teamMessage(phase: "leader" | "discussion" | "summary"): Message {
  return {
    ...message("bot"),
    id: `msg_team_${phase}`,
    metadata: {
      team: {
        session_id: "team_1",
        root_message_id: "msg_root",
        leader_agent_id: "agt_1",
        phase,
        turn: 1
      }
    }
  };
}
