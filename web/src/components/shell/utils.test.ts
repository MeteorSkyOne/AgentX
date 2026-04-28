import { describe, expect, it } from "vitest";
import { conversationActivityLabel } from "./utils";

describe("conversation activity label", () => {
  it("only reports ready when the socket is connected and idle", () => {
    expect(conversationActivityLabel("connected", false)).toBe("ready");
    expect(conversationActivityLabel("connected", true)).toBe("running");
    expect(conversationActivityLabel("connecting", false)).toBe("connecting");
    expect(conversationActivityLabel("reconnecting", false)).toBe("reconnecting");
    expect(conversationActivityLabel("disconnected", false)).toBe("offline");
    expect(conversationActivityLabel("idle", false)).toBe("offline");
  });
});
