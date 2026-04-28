package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/meteorsky/agentx/internal/domain"
	skillpkg "github.com/meteorsky/agentx/internal/skills"
)

type ConversationAgentSkills struct {
	AgentID     string         `json:"agent_id"`
	AgentHandle string         `json:"agent_handle"`
	AgentName   string         `json:"agent_name"`
	Skills      []SkillSummary `json:"skills"`
}

type SkillSummary struct {
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name"`
	Description          string `json:"description"`
	ConflictsWithBuiltin bool   `json:"conflicts_with_builtin"`
}

func (a *App) ConversationSkills(ctx context.Context, conversationType domain.ConversationType, conversationID string) ([]ConversationAgentSkills, error) {
	scope, err := a.conversationScope(ctx, conversationType, conversationID)
	if err != nil {
		return nil, err
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return nil, err
	}

	result := make([]ConversationAgentSkills, 0, len(agents))
	for _, agent := range agents {
		discovered, err := a.discoverSkillsForAgent(agent)
		if err != nil {
			return nil, err
		}
		result = append(result, ConversationAgentSkills{
			AgentID:     agent.Agent.ID,
			AgentHandle: agent.Agent.Handle,
			AgentName:   agent.Agent.Name,
			Skills:      skillSummaries(discovered),
		})
	}
	return result, nil
}

func (a *App) discoverSkillsForAgent(agent ConversationAgentContext) ([]skillpkg.Skill, error) {
	return skillpkg.Discover(skillpkg.DiscoverOptions{
		AgentKind:       agent.Agent.Kind,
		ConfigWorkspace: agent.ConfigWorkspace.Path,
		RunWorkspace:    agent.RunWorkspace.Path,
		Env:             agent.Agent.Env,
		ReservedNames:   builtinSlashCommandNames(),
	})
}

func skillSummaries(discovered []skillpkg.Skill) []SkillSummary {
	summaries := make([]SkillSummary, 0, len(discovered))
	for _, skill := range discovered {
		summaries = append(summaries, SkillSummary{
			Name:                 skill.Name,
			DisplayName:          skill.DisplayName,
			Description:          skill.Description,
			ConflictsWithBuiltin: skill.ConflictsWithBuiltin,
		})
	}
	return summaries
}

func (a *App) handleListSkillsCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext) (domain.Message, error) {
	discovered, err := a.discoverSkillsForAgent(target)
	if err != nil {
		return domain.Message{}, err
	}
	if len(discovered) == 0 {
		return a.createSystemMessage(ctx, req, fmt.Sprintf("No skills found for @%s.", target.Agent.Handle))
	}

	var b strings.Builder
	b.WriteString("Skills for @")
	b.WriteString(target.Agent.Handle)
	b.WriteString(":\n")
	for _, skill := range discovered {
		b.WriteString("- /")
		b.WriteString(skill.Name)
		if skill.Description != "" {
			b.WriteString(" - ")
			b.WriteString(skill.Description)
		}
		if skill.ConflictsWithBuiltin {
			b.WriteString(" (built-in command takes precedence)")
		}
		b.WriteByte('\n')
	}
	return a.createSystemMessage(ctx, req, strings.TrimSpace(b.String()))
}

func (a *App) handleSkillCommand(ctx context.Context, req SendMessageRequest, target ConversationAgentContext, command slashCommand) (domain.Message, error) {
	discovered, err := a.discoverSkillsForAgent(target)
	if err != nil {
		return domain.Message{}, err
	}
	skill, ok := skillpkg.Match(discovered, command.Name)
	if !ok || skill.ConflictsWithBuiltin {
		return domain.Message{}, ErrUnknownCommand
	}
	return a.createCommandRun(ctx, req, target, skillpkg.BuildPrompt(skill, command.Args), "", nil)
}
