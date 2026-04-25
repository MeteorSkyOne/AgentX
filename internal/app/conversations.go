package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

var ErrEmptyMessage = errors.New("empty message")

type SendMessageRequest struct {
	UserID           string
	OrganizationID   string
	ConversationType domain.ConversationType
	ConversationID   string
	Body             string
}

func (a *App) ListOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	return a.store.Organizations().ListForUser(ctx, userID)
}

func (a *App) ListChannels(ctx context.Context, orgID string) ([]domain.Channel, error) {
	return a.store.Channels().ListByOrganization(ctx, orgID)
}

func (a *App) ListMessages(ctx context.Context, conversationType domain.ConversationType, conversationID string, limit int) ([]domain.Message, error) {
	return a.store.Messages().List(ctx, conversationType, conversationID, limit)
}

func (a *App) ConversationBinding(ctx context.Context, conversationType domain.ConversationType, conversationID string) (domain.ConversationBinding, error) {
	return a.store.Bindings().ByConversation(ctx, conversationType, conversationID)
}

func (a *App) SendMessage(ctx context.Context, req SendMessageRequest) (domain.Message, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return domain.Message{}, ErrEmptyMessage
	}

	message := domain.Message{
		ID:               id.New("msg"),
		OrganizationID:   req.OrganizationID,
		ConversationType: req.ConversationType,
		ConversationID:   req.ConversationID,
		SenderType:       domain.SenderUser,
		SenderID:         req.UserID,
		Kind:             domain.MessageText,
		Body:             body,
		CreatedAt:        time.Now().UTC(),
	}
	if err := a.store.Messages().Create(ctx, message); err != nil {
		return domain.Message{}, err
	}

	a.publishConversationEvent(domain.Event{
		Type:             domain.EventMessageCreated,
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Payload:          domain.MessageCreatedPayload{Message: message},
	})

	go a.runAgentForMessage(context.WithoutCancel(ctx), message)

	return message, nil
}
