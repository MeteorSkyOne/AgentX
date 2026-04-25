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
	"nhooyr.io/websocket"
)

type subscribeMessage struct {
	Type             string `json:"type"`
	OrganizationID   string `json:"organization_id"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
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
		OrganizationID: resolvedOrganizationID,
		ConversationID: msg.ConversationID,
	})
	defer unsubscribe()

	if err := writeWebSocketJSON(ctx, conn, map[string]string{"type": "subscribed"}); err != nil {
		return
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				cancel()
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
			if err := writeWebSocketJSON(ctx, conn, event); err != nil {
				_ = conn.Close(websocket.StatusInternalError, "failed to marshal event")
				return
			}
		}
	}
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
