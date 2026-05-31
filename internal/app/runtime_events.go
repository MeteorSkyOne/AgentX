package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

func (a *App) publishAgentRunFailed(message domain.Message, runID string, err error) {
	a.publishAgentRunFailedWithContext(message, runID, "", nil, err)
}

func (a *App) publishAgentRunCanceledWithContext(message domain.Message, runID string, agentID string, team *domain.TeamMetadata) {
	slog.Info(
		"agent run canceled",
		"run_id", runID,
		"agent_id", agentID,
		"organization_id", message.OrganizationID,
		"conversation_type", message.ConversationType,
		"conversation_id", message.ConversationID,
		"message_id", message.ID,
	)
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunCanceled,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.AgentRunPayload{RunID: runID, AgentID: agentID, Team: team},
	})
}

func (a *App) publishAgentRunFailedWithContext(message domain.Message, runID string, agentID string, team *domain.TeamMetadata, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	a.publishAgentRunFailedEvent(message, runID, agentID, team, errText, false)
}

// publishAgentRunFailedEvent emits the ephemeral failure event. When persisted is
// true the same failure has also been stored as a chat message, so clients can
// drop their streaming placeholder and render the persisted message instead.
func (a *App) publishAgentRunFailedEvent(message domain.Message, runID string, agentID string, team *domain.TeamMetadata, errText string, persisted bool) {
	slog.Error(
		"agent run failed",
		"run_id", runID,
		"organization_id", message.OrganizationID,
		"conversation_type", message.ConversationType,
		"conversation_id", message.ConversationID,
		"message_id", message.ID,
		"persisted", persisted,
		"error", errText,
	)
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentRunFailed,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.AgentRunFailedPayload{RunID: runID, AgentID: agentID, Error: errText, Team: team, Persisted: persisted},
	})
}

// isAgentRunCancellation reports whether err is a user- or system-initiated
// cancellation (stop, cancel, context cancellation) rather than a genuine agent
// failure. Cancellations are not persisted as error messages.
func isAgentRunCancellation(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, errAgentRunStopped) ||
		errors.Is(err, errAgentRunCanceled)
}

func (a *App) publishConversationEvent(evt domain.Event) {
	evt.ID = id.New("evt")
	evt.CreatedAt = time.Now().UTC()
	a.bus.Publish(evt)
}

func toAgentInputOptions(opts []agentruntime.InputRequestOption) []domain.AgentInputRequestOption {
	if len(opts) == 0 {
		return nil
	}
	result := make([]domain.AgentInputRequestOption, len(opts))
	for i, o := range opts {
		result[i] = domain.AgentInputRequestOption{Label: o.Label, Description: o.Description}
	}
	return result
}

func runtimeEventError(evt agentruntime.Event) error {
	if evt.Error != "" {
		return errors.New(evt.Error)
	}
	return errors.New("agent runtime failed")
}

func agentRunContextError(ctx context.Context) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return context.Canceled
}

func agentRunLogAttrs(runID string, message domain.Message, agent domain.Agent, workspace string, instructionWorkspace string) []any {
	return []any{
		"run_id", runID,
		"agent_id", agent.ID,
		"agent_kind", agent.Kind,
		"agent_name", agent.Name,
		"model", agent.Model,
		"effort", agent.Effort,
		"fast_mode", agent.FastMode,
		"yolo_mode", agent.YoloMode,
		"organization_id", message.OrganizationID,
		"conversation_type", message.ConversationType,
		"conversation_id", message.ConversationID,
		"message_id", message.ID,
		"workspace", workspace,
		"instruction_workspace", instructionWorkspace,
	}
}

func runtimeProcessItems(evt agentruntime.Event) []domain.ProcessItem {
	if len(evt.Process) == 0 {
		if evt.Thinking == "" {
			return nil
		}
		return []domain.ProcessItem{{
			Type: "thinking",
			Text: evt.Thinking,
		}}
	}
	items := make([]domain.ProcessItem, 0, len(evt.Process))
	for _, item := range evt.Process {
		items = append(items, domain.ProcessItem{
			Type:       item.Type,
			Text:       item.Text,
			ToolName:   item.ToolName,
			ToolCallID: item.ToolCallID,
			Status:     item.Status,
			Input:      item.Input,
			Output:     item.Output,
			Raw:        item.Raw,
			CreatedAt:  item.CreatedAt,
		})
	}
	return items
}
