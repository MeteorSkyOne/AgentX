package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

// Agents can start a turn on their own: Claude Code wakes the main agent to
// report on a background command once it finishes, after the reply that
// launched it is over. The runtime claims such a turn and offers it here; it
// runs as an agent run with no prompt and lands as a reply of its own.

// backgroundTurnReserveTimeout bounds how long a background turn waits for the
// conversation's previous run to wind down before it is dropped.
const backgroundTurnReserveTimeout = 10 * time.Second

const backgroundTurnReserveInterval = 100 * time.Millisecond

func (a *App) registerBackgroundTurnHandlers() {
	for _, rt := range a.opts.Runtimes {
		if notifier, ok := rt.(agentruntime.BackgroundTurnNotifier); ok {
			notifier.SetBackgroundTurnHandler(a.handleBackgroundTurn)
		}
	}
}

func (a *App) handleBackgroundTurn(sessionKey string) bool {
	agentID, conversationType, conversationID, ok := parseAgentSessionKey(sessionKey)
	if !ok {
		return false
	}
	return a.startBackground("agent background turn", func(ctx context.Context) {
		a.runBackgroundTurn(ctx, agentID, conversationType, conversationID)
	})
}

func (a *App) runBackgroundTurn(ctx context.Context, agentID string, conversationType domain.ConversationType, conversationID string) {
	logAttrs := []any{"agent_id", agentID, "conversation_type", conversationType, "conversation_id", conversationID}
	scope, err := a.conversationScope(ctx, conversationType, conversationID)
	if err != nil {
		slog.Warn("agent background turn dropped: conversation lookup failed", append(logAttrs, "error", err)...)
		return
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		slog.Warn("agent background turn dropped: agent lookup failed", append(logAttrs, "error", err)...)
		return
	}
	var target ConversationAgentContext
	for _, candidate := range agents {
		if candidate.Agent.ID == agentID {
			target = candidate
			break
		}
	}
	if target.Agent.ID == "" {
		slog.Warn("agent background turn dropped: agent is no longer in the conversation", logAttrs...)
		return
	}

	// The turn has no user message; this one only carries the conversation it
	// belongs to and is never stored.
	trigger := domain.Message{
		OrganizationID:   scope.organizationID,
		ConversationType: conversationType,
		ConversationID:   conversationID,
		CreatedAt:        time.Now().UTC(),
	}
	opts := agentRunOptions{BackgroundTurn: true}
	reserved := a.newReservedAgentRun(ctx, trigger, target, id.New("run"), opts)
	deadline := time.Now().Add(backgroundTurnReserveTimeout)
	for !a.reserveActiveAgentRunIfIdle(reserved) {
		if !time.Now().Before(deadline) {
			reserved.cancel(nil)
			slog.Warn("agent background turn dropped: conversation is still busy", logAttrs...)
			return
		}
		select {
		case <-time.After(backgroundTurnReserveInterval):
		case <-ctx.Done():
			reserved.cancel(nil)
			return
		}
	}
	a.runReservedAgentForMessageWithTarget(reserved, trigger, target, opts)
}

// agentSessionKey identifies an agent's runtime session in a conversation.
func agentSessionKey(agentID string, conversationType domain.ConversationType, conversationID string) string {
	return agentID + ":" + string(conversationType) + ":" + conversationID
}

func parseAgentSessionKey(key string) (string, domain.ConversationType, string, bool) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], domain.ConversationType(parts[1]), parts[2], true
}
