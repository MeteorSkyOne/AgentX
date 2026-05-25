package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

const runtimeContextMessageLimit = 40
const runtimeContextBodyLimit = 4000

func (a *App) runtimeContextForMessage(ctx context.Context, agentID string, userMessage domain.Message) (string, error) {
	var contextStartedAt *time.Time
	session, err := a.store.Sessions().ByConversation(ctx, agentID, userMessage.ConversationType, userMessage.ConversationID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	} else {
		contextStartedAt = session.ContextStartedAt
	}

	messages, err := a.store.Messages().ListRecent(ctx, userMessage.ConversationType, userMessage.ConversationID, runtimeContextMessageLimit)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, message := range messages {
		if message.ID == userMessage.ID {
			continue
		}
		if message.SenderType == domain.SenderSystem {
			continue
		}
		if contextStartedAt != nil && message.CreatedAt.Before(*contextStartedAt) {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("Conversation history visible to this agent. Use it as context, but reply only to the current user message.\n")
		}
		fmt.Fprintf(
			&b,
			"%s %s: %s\n",
			message.CreatedAt.Format(time.RFC3339),
			runtimeSenderLabel(message),
			runtimeMessageBody(message.Body),
		)
	}
	return strings.TrimSpace(b.String()), nil
}

func (a *App) promptWithReplyReference(ctx context.Context, userMessage domain.Message, prompt string) (string, error) {
	replyToMessageID := strings.TrimSpace(userMessage.ReplyToMessageID)
	if replyToMessageID == "" {
		return prompt, nil
	}

	referenced, err := a.store.Messages().ByID(ctx, replyToMessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deletedReplyReferencePrompt(replyToMessageID, prompt), nil
		}
		return "", err
	}
	if referenced.OrganizationID != userMessage.OrganizationID ||
		referenced.ConversationType != userMessage.ConversationType ||
		referenced.ConversationID != userMessage.ConversationID {
		return deletedReplyReferencePrompt(replyToMessageID, prompt), nil
	}

	var b strings.Builder
	b.WriteString("The current user message is replying to this referenced message.\n")
	fmt.Fprintf(&b, "Referenced message ID: %s\n", referenced.ID)
	fmt.Fprintf(&b, "Referenced sender: %s\n", runtimeSenderLabel(referenced))
	fmt.Fprintf(&b, "Referenced created at: %s\n", referenced.CreatedAt.Format(time.RFC3339))
	b.WriteString("Referenced body:\n")
	b.WriteString(runtimeMessageBody(referenced.Body))
	b.WriteString("\n\nUser message:\n")
	b.WriteString(prompt)
	return b.String(), nil
}

func deletedReplyReferencePrompt(messageID string, prompt string) string {
	return "The current user message is replying to a referenced message, but that referenced message was deleted or is unavailable.\n" +
		"Referenced message ID: " + messageID + "\n\nUser message:\n" + prompt
}

func runtimeSenderLabel(message domain.Message) string {
	switch message.SenderType {
	case domain.SenderUser:
		return "user"
	case domain.SenderBot:
		return "bot:" + message.SenderID
	case domain.SenderSystem:
		return "system"
	default:
		return string(message.SenderType)
	}
}

func runtimeMessageBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "(empty)"
	}
	runes := []rune(body)
	if len(runes) <= runtimeContextBodyLimit {
		return body
	}
	return string(runes[:runtimeContextBodyLimit]) + "\n[truncated]"
}

func joinRuntimeContext(base string, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}
	return base + "\n\n" + extra
}

func runtimeAttachmentsFromMessage(message domain.Message) []agentruntime.Attachment {
	if len(message.Attachments) == 0 {
		return nil
	}
	attachments := make([]agentruntime.Attachment, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		attachments = append(attachments, agentruntime.Attachment{
			ID:          attachment.ID,
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			Kind:        string(attachment.Kind),
			SizeBytes:   attachment.SizeBytes,
			LocalPath:   attachment.StoragePath,
		})
	}
	return attachments
}

func (a *App) firstAgentIDForFailedResolution(ctx context.Context, scope conversationScope) string {
	if scope.legacyBinding != nil {
		return scope.legacyBinding.AgentID
	}
	if scope.channel.ID == "" {
		return ""
	}
	bindings, err := a.store.ChannelAgents().ListByChannel(ctx, scope.channel.ID)
	if err != nil || len(bindings) == 0 {
		return ""
	}
	return bindings[0].AgentID
}

func (a *App) previousProviderSessionID(ctx context.Context, agentID string, message domain.Message) (string, error) {
	session, err := a.store.Sessions().ByConversation(ctx, agentID, message.ConversationType, message.ConversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return session.ProviderSessionID, nil
}
