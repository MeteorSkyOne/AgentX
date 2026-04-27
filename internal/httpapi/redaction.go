package httpapi

import (
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

func redactConversationContext(ctx app.ConversationContext) app.ConversationContext {
	for i := range ctx.Agents {
		ctx.Agents[i].Agent = redactAgent(ctx.Agents[i].Agent)
	}
	ctx.Agent = redactAgent(ctx.Agent)
	return ctx
}

func redactConversationAgents(agents []app.ConversationAgentContext) []app.ConversationAgentContext {
	for i := range agents {
		agents[i].Agent = redactAgent(agents[i].Agent)
	}
	return agents
}

func redactAgent(agent domain.Agent) domain.Agent {
	if len(agent.Env) == 0 {
		agent.Env = map[string]string{}
		return agent
	}
	redacted := make(map[string]string, len(agent.Env))
	for key, value := range agent.Env {
		if value == "" {
			redacted[key] = ""
			continue
		}
		redacted[key] = "********"
	}
	agent.Env = redacted
	return agent
}
