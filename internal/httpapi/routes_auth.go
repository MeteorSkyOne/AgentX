package httpapi

import (
	"errors"
	"net/http"

	"github.com/meteorsky/agentx/internal/app"
)

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var req app.BootstrapRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	result, err := s.app.Bootstrap(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			writeError(w, http.StatusUnauthorized, "unauthorized")
		case errors.Is(err, app.ErrAlreadyBootstrapped):
			writeError(w, http.StatusConflict, "already bootstrapped")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, user)
}
