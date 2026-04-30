package httpapi

import (
	"errors"
	"net/http"

	"github.com/meteorsky/agentx/internal/app"
)

func (s *Server) handleRenderD2Diagram(w http.ResponseWriter, r *http.Request) {
	var req app.D2RenderRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}

	result, err := s.app.RenderD2Diagram(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, app.InvalidInputMessage(err))
		case errors.Is(err, app.ErrD2RenderFailed):
			writeError(w, http.StatusBadRequest, app.D2RenderErrorMessage(err))
		case errors.Is(err, app.ErrD2RenderTimeout):
			writeError(w, http.StatusGatewayTimeout, app.D2RenderErrorMessage(err))
		case errors.Is(err, app.ErrD2CommandUnavailable):
			writeError(w, http.StatusInternalServerError, app.D2RenderErrorMessage(err))
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}
