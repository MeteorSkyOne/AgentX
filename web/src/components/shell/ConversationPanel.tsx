import { useEffect, useMemo, useState } from "react";
import { Composer } from "../Composer";
import { MessagePane } from "../MessagePane";
import type { Channel, ConversationAgentContext, Message, Thread, UserPreferences } from "../../api/types";
import type { ComposerConversation, PendingQuestion, ShellProps, StreamingMessage } from "./types";
import type { ThemeMode } from "../../theme";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
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
  pendingQuestion,
  boundAgents,
  preferences,
  theme,
  composerConversation,
  onSelectThread,
  onCreateThread,
  onUpdateThread,
  onDeleteThread,
  onUpdateMessage,
  onDeleteMessage,
  onLoadOlderMessages,
  onRespondToQuestion,
  onMessageSent,
  workspacePath,
  onOpenWorkspacePath,
}: {
  selectedChannel?: Channel;
  activeThread?: Thread;
  threads: Thread[];
  messages: Message[];
  messagesLoading: boolean;
  olderMessagesLoading: boolean;
  hasOlderMessages: boolean;
  streaming: StreamingMessage[];
  pendingQuestion?: PendingQuestion | null;
  boundAgents: ConversationAgentContext[];
  preferences: UserPreferences;
  theme: ThemeMode;
  composerConversation?: ComposerConversation;
  onSelectThread: ShellProps["onSelectThread"];
  onCreateThread: ShellProps["onCreateThread"];
  onUpdateThread: ShellProps["onUpdateThread"];
  onDeleteThread: ShellProps["onDeleteThread"];
  onUpdateMessage: ShellProps["onUpdateMessage"];
  onDeleteMessage: ShellProps["onDeleteMessage"];
  onLoadOlderMessages: ShellProps["onLoadOlderMessages"];
  onRespondToQuestion?: (questionID: string, answer: string) => Promise<void>;
  onMessageSent: ShellProps["onMessageSent"];
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
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
        pendingQuestion={pendingQuestion}
        agents={boundAgents}
        preferences={preferences}
        theme={theme}
        onUpdateMessage={onUpdateMessage}
        onDeleteMessage={onDeleteMessage}
        onLoadOlder={onLoadOlderMessages}
        onRespondToQuestion={onRespondToQuestion}
        onReplyMessage={selectReplyTarget}
        workspacePath={workspacePath}
        onOpenWorkspacePath={onOpenWorkspacePath}
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
