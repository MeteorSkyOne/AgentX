package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

type userPreferencesUpdateRequest struct {
	ShowTTFT *bool `json:"show_ttft"`
	ShowTPS  *bool `json:"show_tps"`
}

func (s *Server) handleUserPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	preferences, err := s.app.UserPreferences(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (s *Server) handleUpdateUserPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req userPreferencesUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	if req.ShowTTFT == nil || req.ShowTPS == nil {
		writeError(w, http.StatusBadRequest, "missing preference")
		return
	}
	preferences, err := s.app.UpdateUserPreferences(r.Context(), domain.UserPreferences{
		UserID:   userID,
		ShowTTFT: *req.ShowTTFT,
		ShowTPS:  *req.ShowTPS,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (s *Server) handleConversationMetrics(w http.ResponseWriter, r *http.Request) {
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
	filter, ok := parseMetricsFilter(w, r)
	if !ok {
		return
	}
	metrics, err := s.app.ConversationMetrics(r.Context(), conversationType, chi.URLParam(r, "id"), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleChannelMetrics(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	filter, ok := parseMetricsFilter(w, r)
	if !ok {
		return
	}
	metrics, err := s.app.ChannelMetrics(r.Context(), channel.ID, filter)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleProjectMetrics(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	project, ok, err := s.authorizedProject(r, userID, chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	filter, ok := parseMetricsFilter(w, r)
	if !ok {
		return
	}
	metrics, err := s.app.ProjectMetrics(r.Context(), project.ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func parseMetricsFilter(w http.ResponseWriter, r *http.Request) (store.MetricsFilter, bool) {
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return store.MetricsFilter{}, false
		}
		limit = parsed
	}
	provider := r.URL.Query().Get("provider")
	switch provider {
	case "", domain.AgentKindClaude, domain.AgentKindCodex, domain.AgentKindFake:
	default:
		writeError(w, http.StatusBadRequest, "invalid provider")
		return store.MetricsFilter{}, false
	}
	group := r.URL.Query().Get("group")
	switch group {
	case "", "agent":
	default:
		writeError(w, http.StatusBadRequest, "invalid group")
		return store.MetricsFilter{}, false
	}
	return store.MetricsFilter{Limit: limit, Provider: provider, Group: group}, true
}
