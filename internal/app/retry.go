package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/meteorsky/agentx/internal/domain"
)

// ErrNoMessageToRetry is returned when a conversation has no user message that
// could be re-run.
var ErrNoMessageToRetry = errors.New("no message to retry")

// ErrAgentRunInProgress is returned when a retry is requested while the agent
// already has an active run in the conversation.
var ErrAgentRunInProgress = errors.New("agent run already in progress")

// retryHistoryLookback bounds how far back we scan for the latest user message
// when retrying. The trailing user message is normally at (or very near) the
// end of the conversation, so this is comfortably large.
const retryHistoryLookback = 100

// RetryAgentRun re-runs the latest turn for an agent in a conversation: it
// removes the agent's reply to the most recent user message (including a
// persisted failure message) and re-dispatches that user message to the agent.
//
// Only the last turn can be retried; this keeps history non-destructive for
// everything before it. The existing provider session is reused (resumed) so
// the retry preserves prompt-cache continuity. The stale reply may linger in
// the provider's own transcript, but AgentX re-injects the cleaned conversation
// history as context on every turn, and a stale provider session self-heals via
// the in-run retry-without-resume path.
func (a *App) RetryAgentRun(ctx context.Context, conversationType domain.ConversationType, conversationID string, agentID string) error {
	if a.isShuttingDown() {
		return errAppShuttingDown
	}

	scope, err := a.conversationScope(ctx, conversationType, conversationID)
	if err != nil {
		return err
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return err
	}
	var target *ConversationAgentContext
	for i := range agents {
		if agents[i].Agent.ID == agentID {
			target = &agents[i]
			break
		}
	}
	if target == nil {
		return invalidInput("agent is not part of this conversation")
	}

	activeKey := activeRunKey{
		conversationType: conversationType,
		conversationID:   conversationID,
		agentID:          agentID,
	}
	if a.hasActiveAgentRun(activeKey) {
		return ErrAgentRunInProgress
	}

	messages, err := a.store.Messages().ListRecent(ctx, conversationType, conversationID, retryHistoryLookback)
	if err != nil {
		return err
	}
	triggerIndex := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].SenderType == domain.SenderUser {
			triggerIndex = i
			break
		}
	}
	if triggerIndex < 0 {
		return ErrNoMessageToRetry
	}
	triggerMessage := messages[triggerIndex]

	// Remove this agent's reply (or persisted failure) to the trigger message so
	// the stale answer disappears from the UI and from the replayed context. The
	// provider session is intentionally left intact so the resume keeps prompt
	// caching warm; AgentX re-feeds the cleaned history as context on dispatch.
	for _, message := range messages[triggerIndex+1:] {
		if message.SenderType != domain.SenderBot || message.SenderID != target.Agent.BotUserID {
			continue
		}
		if err := a.DeleteMessage(ctx, message.ID); err != nil {
			slog.Warn("retry: failed to delete previous agent reply", "message_id", message.ID, "agent_id", agentID, "error", err)
		}
	}

	slog.Info(
		"retrying agent run",
		"agent_id", agentID,
		"conversation_type", conversationType,
		"conversation_id", conversationID,
		"trigger_message_id", triggerMessage.ID,
	)
	a.dispatchAgentRunOrQueue(context.WithoutCancel(ctx), triggerMessage, *target)
	return nil
}
