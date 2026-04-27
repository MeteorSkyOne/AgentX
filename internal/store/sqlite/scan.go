package sqlite

import (
	"database/sql"
	"encoding/json"

	"github.com/meteorsky/agentx/internal/domain"
)

func scanMessages(rows *sql.Rows) ([]domain.Message, error) {
	var messages []domain.Message
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func scanOrganization(scanner interface {
	Scan(dest ...any) error
}) (domain.Organization, error) {
	var org domain.Organization
	var createdAt string
	if err := scanner.Scan(&org.ID, &org.Name, &createdAt); err != nil {
		return domain.Organization{}, err
	}
	var err error
	org.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Organization{}, err
	}
	return org, nil
}

func scanNotificationSettings(scanner interface {
	Scan(dest ...any) error
}) (domain.NotificationSettings, error) {
	var settings domain.NotificationSettings
	var createdAt, updatedAt string
	var webhookEnabled int
	if err := scanner.Scan(
		&settings.OrganizationID, &webhookEnabled, &settings.WebhookURL, &settings.WebhookSecret,
		&createdAt, &updatedAt,
	); err != nil {
		return domain.NotificationSettings{}, err
	}
	var err error
	settings.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	settings.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	settings.WebhookEnabled = webhookEnabled != 0
	settings.WebhookSecretConfigured = settings.WebhookSecret != ""
	return settings, nil
}

func scanProject(scanner interface {
	Scan(dest ...any) error
}) (domain.Project, error) {
	var project domain.Project
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&project.ID, &project.OrganizationID, &project.Name, &project.WorkspaceID,
		&project.CreatedBy, &createdAt, &updatedAt,
	); err != nil {
		return domain.Project{}, err
	}
	var err error
	project.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	project.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func scanChannel(scanner interface {
	Scan(dest ...any) error
}) (domain.Channel, error) {
	var channel domain.Channel
	var channelType, createdAt, updatedAt string
	var archivedAt sql.NullString
	if err := scanner.Scan(
		&channel.ID, &channel.OrganizationID, &channel.ProjectID, &channelType, &channel.Name,
		&createdAt, &updatedAt, &archivedAt,
	); err != nil {
		return domain.Channel{}, err
	}
	channel.Type = domain.ChannelType(channelType)
	var err error
	channel.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Channel{}, err
	}
	if updatedAt == "" {
		channel.UpdatedAt = channel.CreatedAt
	} else {
		channel.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return domain.Channel{}, err
		}
	}
	channel.ArchivedAt, err = parseNullableTime(archivedAt)
	if err != nil {
		return domain.Channel{}, err
	}
	return channel, nil
}

func scanThread(scanner interface {
	Scan(dest ...any) error
}) (domain.Thread, error) {
	var thread domain.Thread
	var createdAt, updatedAt string
	var archivedAt sql.NullString
	if err := scanner.Scan(
		&thread.ID, &thread.OrganizationID, &thread.ProjectID, &thread.ChannelID, &thread.Title,
		&thread.CreatedBy, &createdAt, &updatedAt, &archivedAt,
	); err != nil {
		return domain.Thread{}, err
	}
	var err error
	thread.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Thread{}, err
	}
	thread.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Thread{}, err
	}
	thread.ArchivedAt, err = parseNullableTime(archivedAt)
	if err != nil {
		return domain.Thread{}, err
	}
	return thread, nil
}

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (domain.Message, error) {
	var message domain.Message
	var conversationType, senderType, kind, metadataJSON, createdAt string
	if err := scanner.Scan(
		&message.ID, &message.OrganizationID, &conversationType, &message.ConversationID,
		&senderType, &message.SenderID, &kind, &message.Body, &metadataJSON, &createdAt,
	); err != nil {
		return domain.Message{}, err
	}
	message.ConversationType = domain.ConversationType(conversationType)
	message.SenderType = domain.SenderType(senderType)
	message.Kind = domain.MessageKind(kind)
	if metadataJSON != "" && metadataJSON != "{}" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metadataJSON), &meta); err == nil && len(meta) > 0 {
			message.Metadata = meta
		}
	}
	var err error
	message.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func scanAgent(scanner interface {
	Scan(dest ...any) error
}) (domain.Agent, error) {
	var agent domain.Agent
	var envJSON, createdAt, updatedAt string
	var enabled, fastMode, yoloMode int
	if err := scanner.Scan(
		&agent.ID, &agent.OrganizationID, &agent.BotUserID, &agent.Kind, &agent.Name, &agent.Handle,
		&agent.Description, &agent.Model, &agent.Effort, &agent.DefaultWorkspaceID, &agent.ConfigWorkspaceID, &enabled, &fastMode, &yoloMode, &envJSON, &createdAt, &updatedAt,
	); err != nil {
		return domain.Agent{}, err
	}
	if err := json.Unmarshal([]byte(envJSON), &agent.Env); err != nil {
		return domain.Agent{}, err
	}
	var err error
	agent.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.Enabled = enabled != 0
	agent.FastMode = fastMode != 0
	agent.YoloMode = yoloMode != 0
	if agent.ConfigWorkspaceID == "" {
		agent.ConfigWorkspaceID = agent.DefaultWorkspaceID
	}
	if agent.DefaultWorkspaceID == "" {
		agent.DefaultWorkspaceID = agent.ConfigWorkspaceID
	}
	return agent, nil
}

func scanAgentSession(scanner interface {
	Scan(dest ...any) error
}) (domain.AgentSession, error) {
	var session domain.AgentSession
	var conversationType, updatedAt string
	var contextStartedAt sql.NullString
	if err := scanner.Scan(
		&session.AgentID, &conversationType, &session.ConversationID, &session.ProviderSessionID,
		&session.Status, &contextStartedAt, &updatedAt,
	); err != nil {
		return domain.AgentSession{}, err
	}
	session.ConversationType = domain.ConversationType(conversationType)
	var err error
	session.ContextStartedAt, err = parseNullableTime(contextStartedAt)
	if err != nil {
		return domain.AgentSession{}, err
	}
	session.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

func emptyMapIfNil[T any](values map[string]T) map[string]T {
	if values == nil {
		return map[string]T{}
	}
	return values
}

func scanWorkspace(scanner interface {
	Scan(dest ...any) error
}) (domain.Workspace, error) {
	var workspace domain.Workspace
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&workspace.ID, &workspace.OrganizationID, &workspace.Type, &workspace.Name, &workspace.Path,
		&workspace.CreatedBy, &createdAt, &updatedAt,
	); err != nil {
		return domain.Workspace{}, err
	}
	var err error
	workspace.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	workspace.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	return workspace, nil
}

func scanChannelAgent(scanner interface {
	Scan(dest ...any) error
}) (domain.ChannelAgent, error) {
	var agent domain.ChannelAgent
	var runWorkspaceID sql.NullString
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&agent.ChannelID, &agent.AgentID, &runWorkspaceID, &createdAt, &updatedAt,
	); err != nil {
		return domain.ChannelAgent{}, err
	}
	if runWorkspaceID.Valid {
		agent.RunWorkspaceID = runWorkspaceID.String
	}
	var err error
	agent.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ChannelAgent{}, err
	}
	agent.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ChannelAgent{}, err
	}
	return agent, nil
}

func scanBinding(scanner interface {
	Scan(dest ...any) error
}) (domain.ConversationBinding, error) {
	var binding domain.ConversationBinding
	var conversationType, createdAt, updatedAt string
	if err := scanner.Scan(
		&binding.ID, &binding.OrganizationID, &conversationType, &binding.ConversationID,
		&binding.AgentID, &binding.WorkspaceID, &createdAt, &updatedAt,
	); err != nil {
		return domain.ConversationBinding{}, err
	}
	binding.ConversationType = domain.ConversationType(conversationType)
	var err error
	binding.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.ConversationBinding{}, err
	}
	binding.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.ConversationBinding{}, err
	}
	return binding, nil
}
