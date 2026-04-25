package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/eventbus"
	"nhooyr.io/websocket"
)

type subscribeMessage struct {
	Type           string `json:"type"`
	OrganizationID string `json:"organization_id"`
	ConversationID string `json:"conversation_id"`
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

	if _, err := s.app.UserForToken(r.Context(), token); err != nil {
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

	_, payload, err := conn.Read(r.Context())
	if err != nil {
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

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events, unsubscribe := s.bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID: msg.OrganizationID,
		ConversationID: msg.ConversationID,
	})
	defer unsubscribe()

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
			payload, err := json.Marshal(event)
			if err != nil {
				_ = conn.Close(websocket.StatusInternalError, "failed to marshal event")
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
				return
			}
		}
	}
}
