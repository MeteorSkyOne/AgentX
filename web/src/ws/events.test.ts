import { describe, expect, it } from "vitest";
import type { SocketEvent } from "./events";
import { isAgentXEvent } from "./events";

describe("isAgentXEvent", () => {
  it("accepts message history events from the socket stream", () => {
    const event: SocketEvent = {
      id: "evt_history_chunk",
      type: "MessageHistoryChunk",
      organization_id: "org_1",
      conversation_type: "channel",
      conversation_id: "chn_1",
      created_at: "2026-04-25T10:00:03Z",
      payload: {
        messages: []
      }
    };

    expect(isAgentXEvent(event)).toBe(true);
  });
});
