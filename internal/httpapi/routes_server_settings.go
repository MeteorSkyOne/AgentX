package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

func (s *Server) handleServerSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}

	settings, err := s.app.ServerSettings(r.Context(), orgID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateServerSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}

	var req app.ServerSettingsUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	settings, err := s.app.UpdateServerSettings(r.Context(), orgID, req)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) authorizeServerSettings(w http.ResponseWriter, r *http.Request, userID string, orgID string) bool {
	role, authorized, err := s.authorizedOrganizationRole(r, userID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return false
	}
	if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return false
	}
	if role != domain.RoleOwner && role != domain.RoleAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}
