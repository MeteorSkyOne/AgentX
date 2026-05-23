package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func (a *App) handleCompactCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, args string) (domain.Message, error) {
	if target.Agent.Kind != domain.AgentKindClaude {
		return a.createSystemMessage(ctx, req, fmt.Sprintf("/compact is not supported for @%s. Use /new to start a fresh context.", target.Agent.Handle))
	}
	prompt := "/compact"
	if args = strings.TrimSpace(args); args != "" {
		prompt += " " + args
	}
	onCompleted := func(runCtx context.Context) error {
		return a.store.Sessions().SetAgentSessionContextStartedAt(runCtx, target.Agent.ID, req.ConversationType, req.ConversationID, time.Now().UTC())
	}
	return a.createCommandRun(ctx, req, target, prompt, "", onCompleted)
}

func (a *App) handleGoalCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, args string) (domain.Message, error) {
	if target.Agent.Kind == domain.AgentKindFake {
		return a.createSystemMessage(ctx, req, fmt.Sprintf("/goal is not supported for @%s.", target.Agent.Handle))
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return domain.Message{}, commandInputError("/goal requires a goal description")
	}
	prompt := "/goal " + args
	return a.createCommandRun(ctx, req, target, prompt, "", nil)
}

func (a *App) handleModelCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, model string) (domain.Message, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return domain.Message{}, commandInputError("/model requires a model name")
	}
	updated, err := a.UpdateAgent(ctx, target.Agent.ID, AgentUpdateRequest{Model: &model})
	if err != nil {
		return domain.Message{}, err
	}
	return a.createSystemMessage(ctx, req, fmt.Sprintf("Updated @%s model to %s.", updated.Handle, updated.Model))
}

func (a *App) handleEffortCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, effort string) (domain.Message, error) {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return domain.Message{}, commandInputError("/effort requires a level")
	}
	updated, err := a.UpdateAgent(ctx, target.Agent.ID, AgentUpdateRequest{Effort: &effort})
	if err != nil {
		return domain.Message{}, err
	}
	return a.createSystemMessage(ctx, req, fmt.Sprintf("Updated @%s effort to %s.", updated.Handle, updated.Effort))
}
