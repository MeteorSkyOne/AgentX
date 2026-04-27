package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/meteorsky/agentx/internal/app"
)

func writeAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, app.InvalidInputMessage(err))
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
