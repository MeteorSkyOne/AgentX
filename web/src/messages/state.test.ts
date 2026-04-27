import { describe, expect, it } from "vitest";
import type { AgentXEvent } from "../ws/events";
import type { Message } from "../api/types";
import {
  eventMatchesActiveConversation,
  mergeMessages,
  messageHistoryLoadingForEvent,
  messageMatchesActiveConversation,
  removeMessageAndMarkReferencesDeleted,
  streamingRunHasCompletedMessage
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

  it("merges history chunks with live events without duplicates", () => {
    const older = message("msg_old", "chn_1", "user", "older", "2026-04-25T10:00:01Z");
    const live = message("msg_live", "chn_1", "user", "live", "2026-04-25T10:00:02Z");

    const result = mergeMessages([live], [older, live]);

    expect(result.map((item) => item.id)).toEqual(["msg_old", "msg_live"]);
  });

  it("refreshes reply previews when the referenced message updates", () => {
    const original = message("msg_original", "chn_1", "user", "original", "2026-04-25T10:00:01Z");
    const reply = {
      ...message("msg_reply", "chn_1", "user", "reply", "2026-04-25T10:00:02Z"),
      reply_to_message_id: original.id,
      reply_to: {
        message_id: original.id,
        sender_type: original.sender_type,
        sender_id: original.sender_id,
        body: original.body,
        created_at: original.created_at
      }
    } satisfies Message;
    const updated = { ...original, body: "updated original" };

    const result = mergeMessages([original, reply], [updated]);

    expect(result.find((item) => item.id === reply.id)?.reply_to?.body).toBe("updated original");
  });
});

describe("removeMessageAndMarkReferencesDeleted", () => {
  it("removes the deleted message and marks loaded reply previews deleted", () => {
    const original = message("msg_original", "chn_1", "user", "original", "2026-04-25T10:00:01Z");
    const reply = {
      ...message("msg_reply", "chn_1", "user", "reply", "2026-04-25T10:00:02Z"),
      reply_to_message_id: original.id,
      reply_to: {
        message_id: original.id,
        sender_type: original.sender_type,
        sender_id: original.sender_id,
        body: original.body,
        created_at: original.created_at
      }
    } satisfies Message;

    const result = removeMessageAndMarkReferencesDeleted([original, reply], original.id);

    expect(result.map((item) => item.id)).toEqual([reply.id]);
    expect(result[0].reply_to).toEqual({
      message_id: original.id,
      deleted: true
    });
  });
});

describe("eventMatchesActiveConversation", () => {
  it("accepts events for the active conversation type and id", () => {
    const event: AgentXEvent = {
      id: "evt_match",
      type: "AgentOutputDelta",
      organization_id: "org_1",
      conversation_type: "thread",
      conversation_id: "thr_1",
      created_at: "2026-04-25T10:00:03Z",
      payload: {
        run_id: "run_1",
        agent_id: "agt_1",
        text: "ok"
      }
    };

    expect(
      eventMatchesActiveConversation(event, {
        organizationID: "org_1",
        conversationType: "thread",
        conversationID: "thr_1"
      })
    ).toBe(true);
  });

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
        conversationType: "channel",
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
        conversationType: "channel",
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
          conversationType: "channel",
          conversationID: "chn_new"
        }
      )
    ).toBe(false);
  });
});

describe("messageHistoryLoadingForEvent", () => {
  it("ends loading when empty history completes", () => {
    const event: AgentXEvent = {
      id: "evt_history_done",
      type: "MessageHistoryCompleted",
      organization_id: "org_1",
      conversation_type: "channel",
      conversation_id: "chn_1",
      created_at: "2026-04-25T10:00:03Z",
      payload: {
        has_more: false
      }
    };

    expect(messageHistoryLoadingForEvent(true, event)).toBe(false);
  });
});

describe("streamingRunHasCompletedMessage", () => {
  it("does not treat older bot messages as completion for a running stream", () => {
    expect(
      streamingRunHasCompletedMessage(
        { agentID: "agt_1", startedAt: "2026-04-25T10:00:05Z" },
        [message("msg_old_bot", "chn_1", "bot", "old reply", "2026-04-25T10:00:02Z", "bot_1")],
        [{ agent: { id: "agt_1", bot_user_id: "bot_1" } }]
      )
    ).toBe(false);
  });

  it("treats a newer bot message from the same agent as stream completion", () => {
    expect(
      streamingRunHasCompletedMessage(
        { agentID: "agt_1", startedAt: "2026-04-25T10:00:05Z" },
        [message("msg_bot", "chn_1", "bot", "done", "2026-04-25T10:00:06Z", "bot_1")],
        [{ agent: { id: "agt_1", bot_user_id: "bot_1" } }]
      )
    ).toBe(true);
  });

  it("ignores newer bot messages from a different agent", () => {
    expect(
      streamingRunHasCompletedMessage(
        { agentID: "agt_1", startedAt: "2026-04-25T10:00:05Z" },
        [message("msg_bot", "chn_1", "bot", "done", "2026-04-25T10:00:06Z", "bot_2")],
        [{ agent: { id: "agt_1", bot_user_id: "bot_1" } }]
      )
    ).toBe(false);
  });
});

function message(
  id: string,
  conversationID: string,
  senderType: Message["sender_type"],
  body: string,
  createdAt: string,
  senderID: string = senderType
): Message {
  return {
    id,
    organization_id: "org_1",
    conversation_type: "channel",
    conversation_id: conversationID,
    sender_type: senderType,
    sender_id: senderID,
    kind: "text",
    body,
    created_at: createdAt
  };
}
