package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type threadCreateRequest struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	InitialBody string `json:"initial_body"`
}

type threadUpdateRequest struct {
	Title string `json:"title"`
}

func (s *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
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
	threads, err := s.app.ListThreads(r.Context(), channel.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, threads)
}

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
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
	var req threadCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	body := req.Body
	if body == "" {
		body = req.InitialBody
	}
	thread, message, err := s.app.CreateThread(r.Context(), userID, channel.ID, req.Title, body)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread, "message": message})
}

func (s *Server) handleUpdateThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	thread, ok, err := s.authorizedThread(r, userID, chi.URLParam(r, "threadID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	var req threadUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateThread(r.Context(), thread.ID, req.Title)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleArchiveThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	thread, ok, err := s.authorizedThread(r, userID, chi.URLParam(r, "threadID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err := s.app.ArchiveThread(r.Context(), thread.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
