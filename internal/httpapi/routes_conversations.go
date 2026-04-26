package httpapi

import (
	"database/sql"
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

type messageUpdateRequest struct {
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
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orgID := chi.URLParam(r, "orgID")
	authorized, err := s.authorizedOrganization(r, userID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}

	channels, err := s.app.ListChannels(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
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
	if _, ok, err := s.authorizedConversationOrganizationID(r, userID, conversationType, chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "conversation not found")
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

func (s *Server) handleConversationContext(w http.ResponseWriter, r *http.Request) {
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
	if _, ok, err := s.authorizedConversationOrganizationID(r, userID, conversationType, chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}

	result, err := s.app.ConversationContext(r.Context(), conversationType, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation context not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, redactConversationContext(result))
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

	orgID, ok, err := s.authorizedConversationOrganizationID(r, userID, conversationType, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "conversation not found")
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
		if errors.Is(err, app.ErrUnknownCommand) {
			writeError(w, http.StatusBadRequest, "unknown command")
			return
		}
		if app.IsCommandInputError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, app.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid input")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, message)
}

func (s *Server) handleUpdateMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	message, ok, err := s.authorizedMessage(r, userID, chi.URLParam(r, "messageID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	var req messageUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	updated, err := s.app.UpdateMessage(r.Context(), message.ID, req.Body)
	if err != nil {
		if errors.Is(err, app.ErrEmptyMessage) {
			writeError(w, http.StatusBadRequest, "empty message")
			return
		}
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	message, ok, err := s.authorizedMessage(r, userID, chi.URLParam(r, "messageID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	if err := s.app.DeleteMessage(r.Context(), message.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authenticatedOrganizations(r *http.Request, userID string) ([]domain.Organization, error) {
	return s.app.ListOrganizations(r.Context(), userID)
}

func (s *Server) authorizedOrganization(r *http.Request, userID string, orgID string) (bool, error) {
	orgs, err := s.authenticatedOrganizations(r, userID)
	if err != nil {
		return false, err
	}
	for _, org := range orgs {
		if org.ID == orgID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) authorizedConversationOrganizationID(r *http.Request, userID string, conversationType domain.ConversationType, conversationID string) (string, bool, error) {
	orgs, err := s.authenticatedOrganizations(r, userID)
	if err != nil {
		return "", false, err
	}

	switch conversationType {
	case domain.ConversationChannel:
		channel, err := s.app.Channel(r.Context(), conversationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", false, nil
			}
			return "", false, err
		}
		if organizationBelongs(orgs, channel.OrganizationID) {
			return channel.OrganizationID, true, nil
		}
		return "", false, nil
	case domain.ConversationThread:
		thread, err := s.app.Thread(r.Context(), conversationID)
		if err == nil {
			if organizationBelongs(orgs, thread.OrganizationID) {
				return thread.OrganizationID, true, nil
			}
			return "", false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
		// Compatibility for legacy bound thread conversations created before the
		// first-class thread table existed.
		fallthrough
	case domain.ConversationDM:
		binding, err := s.app.ConversationBinding(r.Context(), conversationType, conversationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", false, nil
			}
			return "", false, err
		}
		if organizationBelongs(orgs, binding.OrganizationID) {
			return binding.OrganizationID, true, nil
		}
		return "", false, nil
	}
	return "", false, nil
}

func (s *Server) authorizedMessage(r *http.Request, userID string, messageID string) (domain.Message, bool, error) {
	message, err := s.app.Message(r.Context(), messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Message{}, false, nil
		}
		return domain.Message{}, false, err
	}
	if _, ok, err := s.authorizedConversationOrganizationID(
		r,
		userID,
		message.ConversationType,
		message.ConversationID,
	); err != nil || !ok {
		return domain.Message{}, ok, err
	}
	return message, true, nil
}

func organizationBelongs(orgs []domain.Organization, orgID string) bool {
	for _, org := range orgs {
		if org.ID == orgID {
			return true
		}
	}
	return false
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
