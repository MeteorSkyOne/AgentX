package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
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
	req, attachments, err := s.readCreateThreadRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body := req.Body
	if body == "" {
		body = req.InitialBody
	}
	thread, message, err := s.app.CreateThread(r.Context(), userID, channel.ID, req.Title, body, attachments)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread, "message": message})
}

// readCreateThreadRequest parses a thread-creation request as either JSON or,
// when files are attached, multipart/form-data carrying title, body, and files.
func (s *Server) readCreateThreadRequest(w http.ResponseWriter, r *http.Request) (threadCreateRequest, []app.AttachmentUpload, error) {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if strings.EqualFold(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, app.MaxMessageAttachmentTotalBytes+1024*1024)
		if err := r.ParseMultipartForm(app.MaxMessageAttachmentTotalBytes + 1024*1024); err != nil {
			return threadCreateRequest{}, nil, errors.New("malformed multipart form")
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		req := threadCreateRequest{
			Title:       r.FormValue("title"),
			Body:        r.FormValue("body"),
			InitialBody: r.FormValue("initial_body"),
		}
		attachments, err := parseMultipartAttachments(r.MultipartForm)
		if err != nil {
			return threadCreateRequest{}, nil, err
		}
		return req, attachments, nil
	}

	var req threadCreateRequest
	if err := readJSON(r, &req); err != nil {
		return threadCreateRequest{}, nil, errors.New("malformed JSON")
	}
	return req, nil, nil
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
