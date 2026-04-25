import { Bot, CircleAlert, UserRound } from "lucide-react";
import type { Message } from "../api/types";

interface StreamingMessage {
  text: string;
  error?: string;
}

interface MessagePaneProps {
  messages: Message[];
  isLoading: boolean;
  streaming: StreamingMessage | null;
}

export function MessagePane({ messages, isLoading, streaming }: MessagePaneProps) {
  return (
    <section className="message-pane" aria-label="Messages">
      {isLoading ? <div className="empty-state">Loading messages</div> : null}
      {!isLoading && messages.length === 0 && !streaming ? (
        <div className="empty-state">No messages yet</div>
      ) : null}
      <div className="message-stack">
        {messages.map((message) => (
          <article key={message.id} className={`message ${message.sender_type}`}>
            <div className="message-avatar" aria-hidden="true">
              {message.sender_type === "bot" ? <Bot size={16} /> : <UserRound size={16} />}
            </div>
            <div className="message-body">
              <div className="message-meta">
                <span>{senderLabel(message.sender_type)}</span>
                <time dateTime={message.created_at}>{formatTime(message.created_at)}</time>
              </div>
              <p>{message.body}</p>
            </div>
          </article>
        ))}
        {streaming ? (
          <article className={`message ${streaming.error ? "system" : "bot"} streaming`}>
            <div className="message-avatar" aria-hidden="true">
              {streaming.error ? <CircleAlert size={16} /> : <Bot size={16} />}
            </div>
            <div className="message-body">
              <div className="message-meta">
                <span>{streaming.error ? "System" : "Agent"}</span>
              </div>
              <p>{streaming.error ?? streaming.text}</p>
            </div>
          </article>
        ) : null}
      </div>
    </section>
  );
}

function senderLabel(senderType: Message["sender_type"]): string {
  switch (senderType) {
    case "bot":
      return "Agent";
    case "system":
      return "System";
    case "user":
      return "You";
  }
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit"
  }).format(new Date(value));
}
