package sqlite

import "github.com/meteorsky/agentx/internal/domain"

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeChannel(channel domain.Channel) domain.Channel {
	if channel.Type == "" {
		channel.Type = domain.ChannelTypeText
	}
	if channel.TeamMaxBatches <= 0 {
		channel.TeamMaxBatches = 6
	}
	if channel.TeamMaxRuns <= 0 {
		channel.TeamMaxRuns = 12
	}
	if channel.UpdatedAt.IsZero() {
		channel.UpdatedAt = channel.CreatedAt
	}
	return channel
}

func normalizeAgent(agent domain.Agent) domain.Agent {
	if agent.ConfigWorkspaceID == "" {
		agent.ConfigWorkspaceID = agent.DefaultWorkspaceID
	}
	if agent.DefaultWorkspaceID == "" {
		agent.DefaultWorkspaceID = agent.ConfigWorkspaceID
	}
	if agent.Handle == "" {
		agent.Handle = agent.ID
	}
	if !agent.Enabled && agent.CreatedAt.IsZero() {
		agent.Enabled = true
	}
	return agent
}
