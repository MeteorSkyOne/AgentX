import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { Composer } from "../Composer";
import { MessagePane } from "../MessagePane";
import type { Channel, ConversationAgentContext, Message, Thread, UserPreferences } from "../../api/types";
import type { ComposerConversation, PendingQuestion, QueuedPrompt, ShellProps, StreamingMessage } from "./types";
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
  queuedPrompts,
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
  onSteerQueuedPrompt,
  onDeleteQueuedPrompt,
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
  queuedPrompts: QueuedPrompt[];
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
  onSteerQueuedPrompt?: (queueID: string) => Promise<void>;
  onDeleteQueuedPrompt?: (queueID: string) => Promise<void>;
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

  const clearReplyTarget = useCallback(() => {
    setReplyTargetState(null);
  }, []);

  const selectReplyTarget = useCallback((message: Message) => {
    setReplyTargetState((current) => {
      const key = composerConversation ? `${composerConversation.type}:${composerConversation.id}` : "";
      if (!key) return current;
      return { conversationKey: key, message };
    });
  }, [composerConversation]);

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

  const mentionAgents = useMemo(
    () => boundAgents.map((item) => item.agent),
    [boundAgents]
  );

  const typingAgents = useMemo(
    () =>
      streaming
        .filter((s) => !s.error)
        .map((s) => {
          const agent = boundAgents.find((b) => b.agent.id === s.agentID);
          return { name: agent?.agent.name ?? "Agent" };
        }),
    [streaming, boundAgents]
  );

  const handleSent = useCallback(
    (message: Message) => {
      clearReplyTarget();
      onMessageSent(message);
    },
    [clearReplyTarget, onMessageSent]
  );

  if (selectedChannel?.type === "thread" && !activeThread) {
    return (
      <ThreadForum
        threads={threads}
        conversation={composerConversation}
        mentionAgents={boundAgents.map((item) => item.agent)}
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
        conversationKey={conversationKey}
        workspacePath={workspacePath}
        onOpenWorkspacePath={onOpenWorkspacePath}
      />
      <MemoizedComposer
        conversation={composerConversation}
        mentionAgents={mentionAgents}
        replyToMessage={replyTarget}
        onCancelReplyTo={clearReplyTarget}
        typingAgents={typingAgents}
        queuedPrompts={queuedPrompts}
        onSteerQueuedPrompt={onSteerQueuedPrompt}
        onDeleteQueuedPrompt={onDeleteQueuedPrompt}
        onSent={handleSent}
      />
    </>
  );
}

const MemoizedComposer = memo(Composer);
