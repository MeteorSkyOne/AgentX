package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

type sendMessageRequest struct {
	Body string `json:"body"`
}

func (s *Server) handleOrganizations(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orgs, err := s.app.ListOrganizations(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.app.ListChannels(r.Context(), chi.URLParam(r, "orgID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	conversationType, ok := parseConversationType(chi.URLParam(r, "type"))
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown conversation type")
		return
	}

	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	messages, err := s.app.ListMessages(r.Context(), conversationType, chi.URLParam(r, "id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	conversationType, ok := parseConversationType(chi.URLParam(r, "type"))
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown conversation type")
		return
	}

	var req sendMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	orgID, err := s.authenticatedOrganizationID(r, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if orgID == "" {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}

	message, err := s.app.SendMessage(r.Context(), app.SendMessageRequest{
		UserID:           userID,
		OrganizationID:   orgID,
		ConversationType: conversationType,
		ConversationID:   chi.URLParam(r, "id"),
		Body:             req.Body,
	})
	if err != nil {
		if errors.Is(err, app.ErrEmptyMessage) {
			writeError(w, http.StatusBadRequest, "empty message")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, message)
}

func (s *Server) authenticatedOrganizationID(r *http.Request, userID string) (string, error) {
	orgs, err := s.app.ListOrganizations(r.Context(), userID)
	if err != nil {
		return "", err
	}
	if len(orgs) == 0 {
		return "", nil
	}
	return orgs[0].ID, nil
}

func parseConversationType(value string) (domain.ConversationType, bool) {
	switch domain.ConversationType(value) {
	case domain.ConversationChannel:
		return domain.ConversationChannel, true
	case domain.ConversationThread:
		return domain.ConversationThread, true
	case domain.ConversationDM:
		return domain.ConversationDM, true
	default:
		return "", false
	}
}

func parseLimit(r *http.Request) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if limit < 0 {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}
