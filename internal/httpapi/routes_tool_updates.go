package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/config"
)

type toolUpdateRequest struct {
	Tool string `json:"tool"`
}

func (s *Server) handleToolUpdates(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	overview, err := s.app.ToolUpdateOverview(r.Context(), orgID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleUpdateToolUpdateSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	var req config.ToolUpdateSettings
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	overview, err := s.app.UpdateToolUpdateSettings(r.Context(), orgID, req)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleCheckToolUpdates(w http.ResponseWriter, r *http.Request) {
	s.handleToolUpdateAction(w, r, false)
}

func (s *Server) handleRunToolUpdates(w http.ResponseWriter, r *http.Request) {
	s.handleToolUpdateAction(w, r, true)
}

func (s *Server) handleToolUpdateAction(w http.ResponseWriter, r *http.Request, update bool) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if !s.authorizeServerSettings(w, r, userID, orgID) {
		return
	}
	var req toolUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	var (
		overview any
		err      error
	)
	if update {
		overview, err = s.app.StartRunToolUpdate(r.Context(), req.Tool)
	} else {
		overview, err = s.app.CheckToolUpdates(r.Context(), req.Tool)
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
