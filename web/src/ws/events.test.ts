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

  it("accepts message queue events from the socket stream", () => {
    const event: SocketEvent = {
      id: "evt_queue",
      type: "AgentPromptQueued",
      organization_id: "org_1",
      conversation_type: "channel",
      conversation_id: "chn_1",
      created_at: "2026-04-25T10:00:03Z",
      payload: {
        queue_id: "queue_1",
        message_id: "msg_1",
        agent_id: "agt_1",
        body: "next prompt",
        created_at: "2026-04-25T10:00:03Z",
        can_steer: true
      }
    };

    expect(isAgentXEvent(event)).toBe(true);
  });
});
