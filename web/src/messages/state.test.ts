import { describe, expect, it } from "vitest";
import type { AgentXEvent } from "../ws/events";
import type { Message } from "../api/types";
import {
  eventMatchesActiveConversation,
  mergeMessages,
  messageMatchesActiveConversation
} from "./state";

describe("mergeMessages", () => {
  it("keeps newer local websocket messages when older query data arrives", () => {
    const bot = message("msg_bot", "chn_1", "bot", "bot reply", "2026-04-25T10:00:02Z");
    const user = message("msg_user", "chn_1", "user", "hello", "2026-04-25T10:00:01Z");

    expect(mergeMessages([bot], [user]).map((item) => item.id)).toEqual([
      "msg_user",
      "msg_bot"
    ]);
  });

  it("dedupes messages by id", () => {
    const user = message("msg_user", "chn_1", "user", "hello", "2026-04-25T10:00:01Z");

    expect(mergeMessages([user], [user])).toEqual([user]);
  });
});

describe("eventMatchesActiveConversation", () => {
  it("rejects late events from the previous channel", () => {
    const event: AgentXEvent = {
      id: "evt_1",
      type: "AgentOutputDelta",
      organization_id: "org_1",
      conversation_type: "channel",
      conversation_id: "chn_old",
      created_at: "2026-04-25T10:00:03Z",
      payload: {
        run_id: "run_1",
        text: "late"
      }
    };

    expect(
      eventMatchesActiveConversation(event, {
        organizationID: "org_1",
        conversationID: "chn_new"
      })
    ).toBe(false);
  });

  it("rejects message events whose payload belongs to another channel", () => {
    const event: AgentXEvent = {
      id: "evt_2",
      type: "MessageCreated",
      organization_id: "org_1",
      conversation_type: "channel",
      conversation_id: "chn_1",
      created_at: "2026-04-25T10:00:03Z",
      payload: {
        message: message("msg_1", "chn_other", "bot", "wrong channel", "2026-04-25T10:00:03Z")
      }
    };

    expect(
      eventMatchesActiveConversation(event, {
        organizationID: "org_1",
        conversationID: "chn_1"
      })
    ).toBe(false);
  });
});

describe("messageMatchesActiveConversation", () => {
  it("rejects late send responses from another channel", () => {
    expect(
      messageMatchesActiveConversation(
        message("msg_late", "chn_old", "user", "late response", "2026-04-25T10:00:04Z"),
        {
          organizationID: "org_1",
          conversationID: "chn_new"
        }
      )
    ).toBe(false);
  });
});

function message(
  id: string,
  conversationID: string,
  senderType: Message["sender_type"],
  body: string,
  createdAt: string
): Message {
  return {
    id,
    organization_id: "org_1",
    conversation_type: "channel",
    conversation_id: conversationID,
    sender_type: senderType,
    sender_id: senderType,
    kind: "text",
    body,
    created_at: createdAt
  };
}
