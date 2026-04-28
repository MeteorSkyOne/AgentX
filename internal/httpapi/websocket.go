package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/id"
	"nhooyr.io/websocket"
)

const (
	webSocketHistoryLimit     = 100
	webSocketHistoryChunkSize = 25
)

type subscribeMessage struct {
	Type             string `json:"type"`
	OrganizationID   string `json:"organization_id"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	Before           string `json:"before"`
}

type historyLoadRequest struct {
	before time.Time
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := s.app.UserForToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, app.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	subscribeCtx, cancelSubscribe := context.WithTimeout(r.Context(), 5*time.Second)
	_, payload, err := conn.Read(subscribeCtx)
	cancelSubscribe()
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "subscribe message required")
		return
	}

	var msg subscribeMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		_ = conn.Close(websocket.StatusUnsupportedData, "invalid subscribe message")
		return
	}
	if msg.Type != "subscribe" {
		_ = conn.Close(websocket.StatusPolicyViolation, "expected subscribe message")
		return
	}

	conversationType := domain.ConversationChannel
	if msg.ConversationType != "" {
		var ok bool
		conversationType, ok = parseConversationType(msg.ConversationType)
		if !ok {
			_ = conn.Close(websocket.StatusUnsupportedData, "unknown conversation type")
			return
		}
	}

	authorized, err := s.authorizedOrganization(r, user.ID, msg.OrganizationID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "authorization failed")
		return
	}
	if !authorized {
		_ = conn.Close(websocket.StatusPolicyViolation, "unauthorized subscription")
		return
	}

	resolvedOrganizationID, ok, err := s.authorizedConversationOrganizationID(r, user.ID, conversationType, msg.ConversationID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "authorization failed")
		return
	}
	if !ok || resolvedOrganizationID != msg.OrganizationID {
		_ = conn.Close(websocket.StatusPolicyViolation, "unauthorized subscription")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events, unsubscribe := s.bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID:   resolvedOrganizationID,
		ConversationType: conversationType,
		ConversationID:   msg.ConversationID,
	})
	defer unsubscribe()

	if err := writeWebSocketJSON(ctx, conn, map[string]string{"type": "subscribed"}); err != nil {
		return
	}
	if err := s.streamWebSocketMessageHistory(ctx, conn, resolvedOrganizationID, conversationType, msg.ConversationID, nil); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "failed to stream message history")
		return
	}

	readDone := make(chan struct{})
	historyRequests := make(chan historyLoadRequest, 4)
	go func() {
		defer close(readDone)
		for {
			_, payload, err := conn.Read(ctx)
			if err != nil {
				cancel()
				return
			}
			request, ok := parseHistoryLoadRequest(payload, msg, conversationType)
			if !ok {
				_ = conn.Close(websocket.StatusUnsupportedData, "invalid history request")
				cancel()
				return
			}
			select {
			case historyRequests <- request:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		conn.CloseNow()
		<-readDone
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			event = redactEventProcessDetails(event)
			if err := writeWebSocketJSON(ctx, conn, event); err != nil {
				_ = conn.Close(websocket.StatusInternalError, "failed to marshal event")
				return
			}
		case request := <-historyRequests:
			if err := s.streamWebSocketMessageHistory(ctx, conn, resolvedOrganizationID, conversationType, msg.ConversationID, &request.before); err != nil {
				_ = conn.Close(websocket.StatusInternalError, "failed to stream message history")
				return
			}
		}
	}
}

func parseHistoryLoadRequest(payload []byte, subscribed subscribeMessage, subscribedConversationType domain.ConversationType) (historyLoadRequest, bool) {
	var msg subscribeMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return historyLoadRequest{}, false
	}
	if msg.Type != "load_history" {
		return historyLoadRequest{}, false
	}
	if msg.OrganizationID != subscribed.OrganizationID || msg.ConversationID != subscribed.ConversationID {
		return historyLoadRequest{}, false
	}
	conversationType := domain.ConversationChannel
	if msg.ConversationType != "" {
		var ok bool
		conversationType, ok = parseConversationType(msg.ConversationType)
		if !ok {
			return historyLoadRequest{}, false
		}
	}
	if conversationType != subscribedConversationType {
		return historyLoadRequest{}, false
	}
	before, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(msg.Before))
	if err != nil || before.IsZero() {
		return historyLoadRequest{}, false
	}
	return historyLoadRequest{before: before}, true
}

func (s *Server) streamWebSocketMessageHistory(ctx context.Context, conn *websocket.Conn, organizationID string, conversationType domain.ConversationType, conversationID string, before *time.Time) error {
	beforeCursor := ""
	if before != nil {
		beforeCursor = before.UTC().Format(time.RFC3339Nano)
	}
	if err := writeWebSocketJSON(ctx, conn, domain.Event{
		ID:               id.New("evt"),
		Type:             domain.EventMessageHistoryStarted,
		OrganizationID:   organizationID,
		ConversationType: conversationType,
		ConversationID:   conversationID,
		Payload:          domain.MessageHistoryStartedPayload{Before: beforeCursor},
		CreatedAt:        time.Now().UTC(),
	}); err != nil {
		return err
	}

	var messages []domain.Message
	var err error
	if before == nil {
		messages, err = s.app.ListRecentMessages(ctx, conversationType, conversationID, webSocketHistoryLimit+1)
	} else {
		messages, err = s.app.ListRecentMessagesBefore(ctx, conversationType, conversationID, *before, webSocketHistoryLimit+1)
	}
	if err != nil {
		return err
	}
	hasMore := len(messages) > webSocketHistoryLimit
	if hasMore {
		messages = messages[len(messages)-webSocketHistoryLimit:]
	}

	for start := 0; start < len(messages); start += webSocketHistoryChunkSize {
		end := start + webSocketHistoryChunkSize
		if end > len(messages) {
			end = len(messages)
		}
		if err := writeWebSocketJSON(ctx, conn, domain.Event{
			ID:               id.New("evt"),
			Type:             domain.EventMessageHistoryChunk,
			OrganizationID:   organizationID,
			ConversationType: conversationType,
			ConversationID:   conversationID,
			Payload: domain.MessageHistoryChunkPayload{
				Messages: redactMessagesProcessDetails(messages[start:end]),
			},
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
	}

	return writeWebSocketJSON(ctx, conn, domain.Event{
		ID:               id.New("evt"),
		Type:             domain.EventMessageHistoryCompleted,
		OrganizationID:   organizationID,
		ConversationType: conversationType,
		ConversationID:   conversationID,
		Payload: domain.MessageHistoryCompletedPayload{
			HasMore: hasMore,
			Before:  beforeCursor,
		},
		CreatedAt: time.Now().UTC(),
	})
}

func writeWebSocketJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, payload)
}
