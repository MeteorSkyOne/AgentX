import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { ChevronsDown, MessageSquare } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { buildMentionLabels, MentionLabelsContext } from "./message-pane/markdown";
import { groupTeamDiscussionMessages, MessageItem, TeamDiscussionItem } from "./message-pane/MessageItems";
import { StreamingItem } from "./message-pane/ProcessTimeline";
import { QuestionPrompt } from "./message-pane/QuestionPrompt";
import type { MessagePaneProps } from "./message-pane/types";
import { cssEscape, isNearViewportBottom } from "./message-pane/utils";

export type { MessagePaneProps, StreamingMessage } from "./message-pane/types";
export {
  createReadOnlyAttachmentEditorController,
  imageAttachmentPreviewDialogLabel,
  isTextAttachmentPreviewSupported,
  nextImagePreviewPan,
  nextImagePreviewScale,
} from "./message-pane/MessageAttachments";
export { formatMessageTimestamp } from "./message-pane/time";

export function MessagePane({
  messages,
  isLoading,
  isLoadingOlder,
  hasOlderMessages,
  streaming,
  pendingQuestion,
  agents,
  preferences,
  theme,
  onUpdateMessage,
  onDeleteMessage,
  onReplyMessage,
  onRetryMessage,
  onLoadOlder,
  onRespondToQuestion,
  conversationKey,
  workspacePath,
  onOpenWorkspacePath,
}: MessagePaneProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const olderAnchorMessageIDRef = useRef<string | null>(null);
  const shouldStickToBottomRef = useRef(true);
  const previousConversationKeyRef = useRef<string | undefined>(conversationKey);
  const [isAtBottom, setIsAtBottom] = useState(true);
  const agentByBotID = useMemo(() => new Map(agents.map((item) => [item.agent.bot_user_id, item.agent])), [agents]);
  const agentByID = useMemo(() => new Map(agents.map((item) => [item.agent.id, item.agent])), [agents]);
  const messagesByID = useMemo(() => new Map(messages.map((message) => [message.id, message])), [messages]);
  const messageItems = useMemo(() => groupTeamDiscussionMessages(messages), [messages]);
  const mentionLabels = useMemo(() => buildMentionLabels(agents), [agents]);
  // Only the trailing agent reply can be retried in place, and only when no run
  // is currently streaming for the conversation.
  const lastMessage = messages.length > 0 ? messages[messages.length - 1] : undefined;
  const retryableMessageID =
    onRetryMessage && streaming.length === 0 && lastMessage?.sender_type === "bot"
      ? lastMessage.id
      : undefined;
  const hasRenderableItems = messages.length > 0 || streaming.length > 0 || Boolean(pendingQuestion);
  const showInitialLoading = isLoading && !hasRenderableItems;

  useLayoutEffect(() => {
    const conversationChanged = previousConversationKeyRef.current !== conversationKey;
    if (conversationChanged) {
      previousConversationKeyRef.current = conversationKey;
      olderAnchorMessageIDRef.current = null;
      shouldStickToBottomRef.current = true;
      setIsAtBottom(true);
    }

    const anchorID = olderAnchorMessageIDRef.current;
    if (anchorID) {
      const viewport = viewportRef.current;
      const anchor = viewport?.querySelector<HTMLElement>(
        `[data-message-id="${cssEscape(anchorID)}"]`
      );
      anchor?.scrollIntoView({ block: "start" });
      if (!isLoadingOlder) {
        olderAnchorMessageIDRef.current = null;
      }
      return;
    }

    if (shouldStickToBottomRef.current) {
      bottomRef.current?.scrollIntoView({ block: "end" });
      setIsAtBottom(true);
    }
  }, [messages, streaming, isLoadingOlder, conversationKey]);

  function handleScroll() {
    const viewport = viewportRef.current;
    if (!viewport) {
      return;
    }

    const nearBottom = isNearViewportBottom(viewport);
    shouldStickToBottomRef.current = nearBottom;
    setIsAtBottom(nearBottom);

    if (
      viewport.scrollTop > 80 ||
      isLoading ||
      isLoadingOlder ||
      !hasOlderMessages ||
      messages.length === 0
    ) {
      return;
    }

    olderAnchorMessageIDRef.current = messages[0].id;
    if (!onLoadOlder()) {
      olderAnchorMessageIDRef.current = null;
    }
  }

  function scrollToBottom() {
    shouldStickToBottomRef.current = true;
    setIsAtBottom(true);
    bottomRef.current?.scrollIntoView({ block: "end", behavior: "smooth" });
  }

  function jumpToMessage(messageID: string) {
    const viewport = viewportRef.current;
    const target = viewport?.querySelector<HTMLElement>(
      `[data-message-id="${cssEscape(messageID)}"]`
    );
    target?.scrollIntoView({ block: "center" });
  }

  if (showInitialLoading) {
    return (
      <section className="flex min-h-0 flex-1 items-center justify-center">
        <span className="text-sm text-muted-foreground">Loading messages...</span>
      </section>
    );
  }

  if (!hasRenderableItems) {
    return (
      <section className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3">
        <MessageSquare className="h-12 w-12 text-muted-foreground" />
        <span className="text-sm text-muted-foreground">No messages yet</span>
      </section>
    );
  }

  return (
    <MentionLabelsContext.Provider value={mentionLabels}>
      <div className="relative min-h-0 min-w-0 flex-1">
        <ScrollArea
          className="h-full min-h-0 min-w-0"
          aria-label="Messages"
          viewportRef={viewportRef}
          viewportClassName="[&>div]:!block [&>div]:!min-w-0 [&>div]:!w-full [&>div]:!max-w-full"
          onViewportScroll={handleScroll}
        >
          <section className="min-w-0 max-w-full p-3 md:p-4">
            <div className="min-w-0 max-w-full space-y-4">
              {isLoadingOlder && (
                <div className="py-2 text-center text-xs text-muted-foreground">
                  Loading older messages...
                </div>
              )}
              {messageItems.map((item) => {
                if (item.type === "team") {
                  return (
                    <TeamDiscussionItem
                      key={`team:${item.sessionID}`}
                      messages={item.messages}
                      agentByBotID={agentByBotID}
                      messagesByID={messagesByID}
                      preferences={preferences}
                      onUpdateMessage={onUpdateMessage}
                      onDeleteMessage={onDeleteMessage}
                      onReplyMessage={onReplyMessage}
                      onJumpToReplyMessage={jumpToMessage}
                      theme={theme}
                      workspacePath={workspacePath}
                      onOpenWorkspacePath={onOpenWorkspacePath}
                    />
                  );
                }
                const message = item.message;
                const agent = agentByBotID.get(message.sender_id);
                const replyAgent =
                  message.reply_to?.sender_type === "bot"
                    ? agentByBotID.get(message.reply_to.sender_id ?? "")
                    : undefined;
                return (
                  <MessageItem
                    key={message.id}
                    message={message}
                    agentName={agent?.name}
                    agentKind={agent?.kind}
                    agentID={agent?.id}
                    replyAgentName={replyAgent?.name}
                    replyTargetLoaded={Boolean(
                      message.reply_to && messagesByID.has(message.reply_to.message_id)
                    )}
                    preferences={preferences}
                    onUpdateMessage={onUpdateMessage}
                    onDeleteMessage={onDeleteMessage}
                    onReplyMessage={onReplyMessage}
                    onRetryMessage={onRetryMessage}
                    canRetry={message.id === retryableMessageID}
                    onJumpToReplyMessage={jumpToMessage}
                    theme={theme}
                    workspacePath={workspacePath}
                    onOpenWorkspacePath={onOpenWorkspacePath}
                  />
                );
              })}
              {streaming.map((item) => {
                const agent = agentByID.get(item.agentID ?? "");
                return (
                  <StreamingItem
                    key={item.runID}
                    item={item}
                    agentName={agent?.name}
                    agentKind={agent?.kind}
                    agentID={agent?.id}
                    hideAvatar={preferences.hide_avatars}
                    workspacePath={workspacePath}
                    onOpenWorkspacePath={onOpenWorkspacePath}
                  />
                );
              })}
              {pendingQuestion && onRespondToQuestion && (
                <QuestionPrompt
                  question={pendingQuestion}
                  agentName={agentByID.get(pendingQuestion.agentID)?.name}
                  agentKind={agentByID.get(pendingQuestion.agentID)?.kind}
                  agentID={pendingQuestion.agentID}
                  hideAvatar={preferences.hide_avatars}
                  onSubmit={(answer) => onRespondToQuestion(pendingQuestion.questionID, answer)}
                />
              )}
              <div ref={bottomRef} />
            </div>
          </section>
        </ScrollArea>
        {!isAtBottom && (
          <Button
            type="button"
            size="icon"
            variant="secondary"
            className="absolute right-4 bottom-4 z-10 h-9 w-9 rounded-full border border-border shadow-md"
            title="Scroll to bottom"
            aria-label="Scroll to bottom"
            onClick={scrollToBottom}
          >
            <ChevronsDown className="h-4 w-4" />
          </Button>
        )}
      </div>
    </MentionLabelsContext.Provider>
  );
}
