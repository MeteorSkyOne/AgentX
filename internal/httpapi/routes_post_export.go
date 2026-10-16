package httpapi

import (
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/transcript"
)

func (s *Server) handleExportThread(w http.ResponseWriter, r *http.Request) {
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
	query := r.URL.Query()
	format := query.Get("format")
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "json" {
		writeError(w, http.StatusBadRequest, "format must be markdown or json")
		return
	}
	post, err := s.app.ExportThread(r.Context(), thread.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if format == "json" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": post.ID + ".json"}))
		writeJSON(w, http.StatusOK, post)
		return
	}
	body := transcript.Markdown(post, transcript.RenderOptions{IncludeProcess: query.Get("process") == "1"})
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": post.ID + ".md"}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleMCPSetup(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.app.MCPSetup())
}
