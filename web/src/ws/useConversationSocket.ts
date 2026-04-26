import { useCallback, useEffect, useRef } from "react";
import { getToken } from "../api/client";
import type { ConversationType } from "../api/types";
import type { AgentXEvent, SocketEvent } from "./events";
import { isAgentXEvent } from "./events";

const maxReconnectAttempts = 5;
const baseReconnectDelayMS = 300;
const maxReconnectDelayMS = 5000;

export function useConversationSocket(
  organizationID: string | undefined,
  conversationType: ConversationType | undefined,
  conversationID: string | undefined,
  onEvent: (event: AgentXEvent) => void
): (before: string) => boolean {
  const onEventRef = useRef(onEvent);
  const socketRef = useRef<WebSocket | null>(null);
  const token = getToken();

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    if (!organizationID || !conversationType || !conversationID || !token) {
      return;
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = new URL("/api/ws", `${protocol}//${window.location.host}`);
    url.searchParams.set("token", token);

    let stopped = false;
    let reconnectAttempts = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

    function connect() {
      if (stopped) {
        return;
      }

      const activeSocket = new WebSocket(url);
      socketRef.current = activeSocket;

      activeSocket.addEventListener("open", () => {
        activeSocket.send(
          JSON.stringify({
            type: "subscribe",
            organization_id: organizationID,
            conversation_type: conversationType,
            conversation_id: conversationID
          })
        );
      });

      activeSocket.addEventListener("message", (message) => {
        try {
          const event = JSON.parse(message.data as string) as SocketEvent;
          if (isAgentXEvent(event)) {
            onEventRef.current(event);
          }
        } catch {
          // Ignore malformed WebSocket messages; the server closes invalid protocol flows.
        }
      });

      activeSocket.addEventListener("close", scheduleReconnect);
      activeSocket.addEventListener("error", () => {
        scheduleReconnect();
        activeSocket.close();
      });
    }

    function scheduleReconnect() {
      if (stopped || reconnectTimer || reconnectAttempts >= maxReconnectAttempts) {
        return;
      }

      const delay = Math.min(
        baseReconnectDelayMS * 2 ** reconnectAttempts,
        maxReconnectDelayMS
      );
      reconnectAttempts += 1;
      reconnectTimer = setTimeout(() => {
        reconnectTimer = undefined;
        connect();
      }, delay);
    }

    connect();

    return () => {
      stopped = true;
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
      }
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [organizationID, conversationType, conversationID, token]);

  return useCallback(
    (before: string): boolean => {
      if (!organizationID || !conversationType || !conversationID) {
        return false;
      }
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        return false;
      }
      socket.send(
        JSON.stringify({
          type: "load_history",
          organization_id: organizationID,
          conversation_type: conversationType,
          conversation_id: conversationID,
          before
        })
      );
      return true;
    },
    [organizationID, conversationType, conversationID]
  );
}
