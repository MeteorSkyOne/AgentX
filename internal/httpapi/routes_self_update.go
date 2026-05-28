package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/config"
)

func (s *Server) handleSelfUpdate(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	overview, err := s.app.SelfUpdateOverview(r.Context(), orgID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleUpdateSelfUpdateSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	var req config.SelfUpdateSettings
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	overview, err := s.app.UpdateSelfUpdateSettings(r.Context(), orgID, req)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleCheckSelfUpdate(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	overview, err := s.app.CheckSelfUpdate(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleRunSelfUpdate(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	overview, err := s.app.StartRunSelfUpdate(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
