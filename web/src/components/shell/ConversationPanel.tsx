import { useEffect, useMemo, useState } from "react";
import { Composer } from "../Composer";
import { MessagePane } from "../MessagePane";
import type { Channel, ConversationAgentContext, Message, Thread } from "../../api/types";
import type { ComposerConversation, ShellProps, StreamingMessage } from "./types";
import { ThreadForum } from "./ThreadForum";

export function ConversationPanel({
  selectedChannel,
  activeThread,
  threads,
  messages,
  messagesLoading,
  olderMessagesLoading,
  hasOlderMessages,
  streaming,
  boundAgents,
  composerConversation,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onMessageSent,
}: {
  selectedChannel?: Channel;
  activeThread?: Thread;
  threads: Thread[];
  messages: Message[];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  boundAgents: ConversationAgentContext[];
  composerConversation?: ComposerConversation;
  onSelectThread: ShellProps["onSelectThread"];
  onCreateThread: ShellProps["onCreateThread"];
  onUpdateThread: ShellProps["onUpdateThread"];
  onDeleteThread: ShellProps["onDeleteThread"];
  onUpdateMessage: ShellProps["onUpdateMessage"];
  onDeleteMessage: ShellProps["onDeleteMessage"];
  onLoadOlderMessages: ShellProps["onLoadOlderMessages"];
  onMessageSent: ShellProps["onMessageSent"];
}) {
  const [replyTargetState, setReplyTargetState] = useState<{
    conversationKey: string;
    message: Message;
  } | null>(null);
  const conversationKey = useMemo(
    () => (composerConversation ? `${composerConversation.type}:${composerConversation.id}` : ""),
    [composerConversation]
  );
  const replyTarget =
    replyTargetState?.conversationKey === conversationKey ? replyTargetState.message : null;

  function clearReplyTarget() {
    setReplyTargetState(null);
  }

  function selectReplyTarget(message: Message) {
    if (!conversationKey) {
      return;
    }
    setReplyTargetState({ conversationKey, message });
  }

  useEffect(() => {
    clearReplyTarget();
  }, [conversationKey]);

  useEffect(() => {
    if (!replyTarget) {
      return;
    }
    const currentTarget = messages.find((message) => message.id === replyTarget.id);
    if (!currentTarget) {
      clearReplyTarget();
      return;
    }
    if (currentTarget !== replyTarget) {
      setReplyTargetState({ conversationKey, message: currentTarget });
    }
  }, [conversationKey, messages, replyTarget, replyTarget?.id]);

  if (selectedChannel?.type === "thread" && !activeThread) {
    return (
      <ThreadForum
        threads={threads}
        onSelectThread={onSelectThread}
        onCreateThread={onCreateThread}
        onUpdateThread={onUpdateThread}
        onDeleteThread={onDeleteThread}
      />
    );
  }

  return (
    <>
      <MessagePane
        messages={messages}
        isLoading={messagesLoading}
        isLoadingOlder={olderMessagesLoading}
        hasOlderMessages={hasOlderMessages}
        streaming={streaming}
        agents={boundAgents}
        onUpdateMessage={onUpdateMessage}
        onDeleteMessage={onDeleteMessage}
        onLoadOlder={onLoadOlderMessages}
        onReplyMessage={selectReplyTarget}
      />
      <Composer
        conversation={composerConversation}
        mentionAgents={boundAgents.map((item) => item.agent)}
        replyToMessage={replyTarget}
        onCancelReplyTo={clearReplyTarget}
        typingAgents={streaming
          .filter((s) => !s.error)
          .map((s) => {
            const agent = boundAgents.find((b) => b.agent.id === s.agentID);
            return { name: agent?.agent.name ?? "Agent" };
          })}
        onSent={(message) => {
          clearReplyTarget();
          onMessageSent(message);
        }}
      />
    </>
  );
}
