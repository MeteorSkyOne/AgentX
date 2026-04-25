import { useEffect, useRef } from "react";
import { getToken } from "../api/client";
import type { AgentXEvent, SocketEvent } from "./events";
import { isAgentXEvent } from "./events";

export function useConversationSocket(
  organizationID: string | undefined,
  conversationID: string | undefined,
  onEvent: (event: AgentXEvent) => void
): void {
  const onEventRef = useRef(onEvent);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    const token = getToken();
    if (!organizationID || !conversationID || !token) {
      return;
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = new URL("/api/ws", `${protocol}//${window.location.host}`);
    url.searchParams.set("token", token);

    const socket = new WebSocket(url);

    socket.addEventListener("open", () => {
      socket.send(
        JSON.stringify({
          type: "subscribe",
          organization_id: organizationID,
          conversation_type: "channel",
          conversation_id: conversationID
        })
      );
    });

    socket.addEventListener("message", (message) => {
      try {
        const event = JSON.parse(message.data as string) as SocketEvent;
        if (isAgentXEvent(event)) {
          onEventRef.current(event);
        }
      } catch {
        // Ignore malformed WebSocket messages; the server closes invalid protocol flows.
      }
    });

    return () => {
      socket.close();
    };
  }, [organizationID, conversationID]);
}
