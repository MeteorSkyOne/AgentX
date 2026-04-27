package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
)

type notificationSettingsRequest struct {
	WebhookEnabled bool    `json:"webhook_enabled"`
	WebhookURL     string  `json:"webhook_url"`
	WebhookSecret  *string `json:"webhook_secret"`
}

type notificationTestResponse struct {
	OK bool `json:"ok"`
}

func (s *Server) handleNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	settings, err := s.app.NotificationSettings(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	var req notificationSettingsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	settings, err := s.app.UpdateNotificationSettings(r.Context(), orgID, app.NotificationSettingsUpdateRequest{
		WebhookEnabled: req.WebhookEnabled,
		WebhookURL:     req.WebhookURL,
		WebhookSecret:  req.WebhookSecret,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleTestNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	if err := s.app.TestNotificationSettings(r.Context(), orgID); err != nil {
		if errors.Is(err, app.ErrWebhookDeliveryFailed) {
			writeError(w, http.StatusBadGateway, "webhook delivery failed")
			return
		}
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notificationTestResponse{OK: true})
}
