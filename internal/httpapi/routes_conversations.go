package httpapi

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

type sendMessageRequest struct {
	Body             string `json:"body"`
	ReplyToMessageID string `json:"reply_to_message_id"`
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
	writeJSON(w, http.StatusOK, redactMessagesProcessDetails(messages))
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

func (s *Server) handleConversationSkills(w http.ResponseWriter, r *http.Request) {
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

	result, err := s.app.ConversationSkills(r.Context(), conversationType, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation skills not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, result)
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

	req, attachments, err := s.readSendMessageRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
		ReplyToMessageID: req.ReplyToMessageID,
		Attachments:      attachments,
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
			writeError(w, http.StatusBadRequest, app.InvalidInputMessage(err))
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, redactMessageProcessDetails(message))
}

func (s *Server) readSendMessageRequest(w http.ResponseWriter, r *http.Request) (sendMessageRequest, []app.AttachmentUpload, error) {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if strings.EqualFold(contentType, "multipart/form-data") {
		return readMultipartSendMessageRequest(w, r)
	}

	var req sendMessageRequest
	if err := readJSON(r, &req); err != nil {
		return sendMessageRequest{}, nil, errors.New("malformed JSON")
	}
	return req, nil, nil
}

func readMultipartSendMessageRequest(w http.ResponseWriter, r *http.Request) (sendMessageRequest, []app.AttachmentUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, app.MaxMessageAttachmentTotalBytes+1024*1024)
	if err := r.ParseMultipartForm(app.MaxMessageAttachmentTotalBytes + 1024*1024); err != nil {
		return sendMessageRequest{}, nil, errors.New("malformed multipart form")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	req := sendMessageRequest{
		Body:             r.FormValue("body"),
		ReplyToMessageID: r.FormValue("reply_to_message_id"),
	}
	var headers []*multipart.FileHeader
	if r.MultipartForm != nil {
		headers = append(headers, r.MultipartForm.File["files[]"]...)
		headers = append(headers, r.MultipartForm.File["files"]...)
	}
	attachments := make([]app.AttachmentUpload, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			slog.Warn("failed to open uploaded attachment", "filename", header.Filename, "error", err)
			return sendMessageRequest{}, nil, errors.New("failed to read attachment")
		}
		data, readErr := io.ReadAll(io.LimitReader(file, app.MaxAttachmentBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			slog.Warn("failed to read uploaded attachment", "filename", header.Filename, "error", readErr)
			return sendMessageRequest{}, nil, errors.New("failed to read attachment")
		}
		if closeErr != nil {
			slog.Warn("failed to close uploaded attachment", "filename", header.Filename, "error", closeErr)
			return sendMessageRequest{}, nil, errors.New("failed to read attachment")
		}
		if int64(len(data)) > app.MaxAttachmentBytes {
			return sendMessageRequest{}, nil, errors.New("attachment exceeds 10 MiB")
		}
		attachments = append(attachments, app.AttachmentUpload{
			Filename:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Data:        data,
		})
	}
	return req, attachments, nil
}

func (s *Server) handleAttachmentContent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	attachment, err := s.app.Attachment(r.Context(), chi.URLParam(r, "attachmentID"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if orgID, ok, err := s.authorizedConversationOrganizationID(r, userID, attachment.ConversationType, attachment.ConversationID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !ok || orgID != attachment.OrganizationID {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	file, err := os.Open(attachment.StoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "attachment content not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": attachment.Filename}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", stat.ModTime(), file)
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
	writeJSON(w, http.StatusOK, redactMessageProcessDetails(updated))
}

func (s *Server) handleMessageProcessItem(w http.ResponseWriter, r *http.Request) {
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

	index, ok := parseProcessIndex(chi.URLParam(r, "index"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid process item index")
		return
	}
	detail, ok := messageProcessDetail(message, index)
	if !ok {
		writeError(w, http.StatusNotFound, "process item not found")
		return
	}
	writeJSON(w, http.StatusOK, detail)
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
