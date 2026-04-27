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
      />
      <Composer
        conversation={composerConversation}
        mentionAgents={boundAgents.map((item) => item.agent)}
        typingAgents={streaming
          .filter((s) => !s.error)
          .map((s) => {
            const agent = boundAgents.find((b) => b.agent.id === s.agentID);
            return { name: agent?.agent.name ?? "Agent" };
          })}
        onSent={onMessageSent}
      />
    </>
  );
}
