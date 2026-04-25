import { FormEvent, useState } from "react";
import { SendHorizonal } from "lucide-react";
import { sendMessage } from "../api/client";
import type { Channel, Message } from "../api/types";

interface ComposerProps {
  channel?: Channel;
  onSent: (message: Message) => void;
}

export function Composer({ channel, onSent }: ComposerProps) {
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const trimmed = body.trim();

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!channel || trimmed === "") {
      return;
    }

    setError(null);
    setSubmitting(true);
    try {
      const message = await sendMessage("channel", channel.id, trimmed);
      setBody("");
      onSent(message);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Message failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="composer" onSubmit={handleSubmit}>
      <div className="composer-row">
        <input
          value={body}
          onChange={(event) => setBody(event.target.value)}
          disabled={!channel || submitting}
          placeholder={channel ? `Message #${channel.name}` : "Select a channel"}
          aria-label="Message"
        />
        <button
          className="icon-button send-button"
          type="submit"
          title="Send"
          aria-label="Send"
          disabled={!channel || submitting || trimmed === ""}
        >
          <SendHorizonal size={18} />
        </button>
      </div>
      {error ? <div className="composer-error">{error}</div> : null}
    </form>
  );
}
