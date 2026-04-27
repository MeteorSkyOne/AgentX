package httpapi

import (
	"errors"
	"net/http"

	"github.com/meteorsky/agentx/internal/app"
)

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.AuthStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleSetupAdmin(w http.ResponseWriter, r *http.Request) {
	var req app.SetupAdminRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	result, err := s.app.SetupAdmin(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			writeError(w, http.StatusUnauthorized, "unauthorized")
		case errors.Is(err, app.ErrAlreadyBootstrapped):
			writeError(w, http.StatusConflict, "already bootstrapped")
		case errors.Is(err, app.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, app.InvalidInputMessage(err))
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req app.LoginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	result, err := s.app.Login(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			writeError(w, http.StatusUnauthorized, "unauthorized")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := s.app.Logout(r.Context(), token); err != nil {
		if errors.Is(err, app.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, user)
}
