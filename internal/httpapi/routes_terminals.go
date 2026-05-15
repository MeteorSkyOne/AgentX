package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
	"nhooyr.io/websocket"
)

const terminalWebSocketReadLimit = 128 * 1024

type terminalClientMessage struct {
	Type       string `json:"type"`
	TerminalID string `json:"terminal_id"`
	ClientID   string `json:"client_id"`
	Data       string `json:"data"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	SinceSeq   uint64 `json:"since_seq"`
}

type terminalRenameRequest struct {
	Title string `json:"title"`
}

func (s *Server) handleWorkspaceTerminals(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, ok, err := s.authorizedWorkspace(r, userID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if !s.authorizedTerminalAccess(w, r, userID, workspace.OrganizationID) {
		return
	}
	writeJSON(w, http.StatusOK, s.app.ListTerminalSessions(r.Context(), userID, workspace))
}

func (s *Server) handleRenameWorkspaceTerminal(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, ok, err := s.authorizedWorkspace(r, userID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if !s.authorizedTerminalAccess(w, r, userID, workspace.OrganizationID) {
		return
	}

	var req terminalRenameRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.RenameTerminal(r.Context(), app.TerminalRenameRequest{
		UserID:     userID,
		Workspace:  workspace,
		TerminalID: chi.URLParam(r, "terminalID"),
		Title:      req.Title,
	})
	if err != nil {
		writeTerminalHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteWorkspaceTerminal(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, ok, err := s.authorizedWorkspace(r, userID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if !s.authorizedTerminalAccess(w, r, userID, workspace.OrganizationID) {
		return
	}
	if err := s.app.TerminateTerminal(r.Context(), userID, workspace, chi.URLParam(r, "terminalID")); err != nil {
		writeTerminalHTTPError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWorkspaceTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	user, ok := s.userForWebSocketToken(w, r)
	if !ok {
		return
	}
	workspace, authorized, err := s.authorizedWorkspace(r, user.ID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !authorized {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if !s.authorizedTerminalAccess(w, r, user.ID, workspace.OrganizationID) {
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(terminalWebSocketReadLimit)

	subscribeCtx, cancelSubscribe := context.WithTimeout(r.Context(), 5*time.Second)
	typ, payload, err := conn.Read(subscribeCtx)
	cancelSubscribe()
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "attach message required")
		return
	}
	if typ != websocket.MessageText {
		_ = conn.Close(websocket.StatusUnsupportedData, "terminal protocol requires text frames")
		return
	}
	var attach terminalClientMessage
	if err := json.Unmarshal(payload, &attach); err != nil || attach.Type != "attach" {
		_ = conn.Close(websocket.StatusUnsupportedData, "invalid attach message")
		return
	}

	attachment, err := s.app.AttachTerminal(r.Context(), app.TerminalAttachRequest{
		UserID:     user.ID,
		Workspace:  workspace,
		TerminalID: strings.TrimSpace(attach.TerminalID),
		ClientID:   strings.TrimSpace(attach.ClientID),
		Cols:       attach.Cols,
		Rows:       attach.Rows,
		SinceSeq:   attach.SinceSeq,
	})
	if err != nil {
		_ = writeWebSocketJSON(r.Context(), conn, terminalErrorFrame(err))
		_ = conn.Close(websocket.StatusPolicyViolation, "terminal attach failed")
		return
	}
	defer attachment.Detach()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if err := writeWebSocketJSON(ctx, conn, app.TerminalFrame{
		Type:       "ready",
		TerminalID: attachment.Session.ID,
		Session:    &attachment.Session,
	}); err != nil {
		return
	}
	if attachment.HistoryTruncated {
		if err := writeWebSocketJSON(ctx, conn, app.TerminalFrame{
			Type:       "history_truncated",
			TerminalID: attachment.Session.ID,
			Truncated:  true,
		}); err != nil {
			return
		}
	}
	for _, frame := range attachment.History {
		if err := writeWebSocketJSON(ctx, conn, frame); err != nil {
			return
		}
	}
	if attachment.Session.Status == "exited" {
		_ = writeWebSocketJSON(ctx, conn, app.TerminalFrame{
			Type:       "exit",
			TerminalID: attachment.Session.ID,
			ExitCode:   attachment.Session.ExitCode,
			Error:      attachment.Session.Error,
		})
		return
	}

	readErr := make(chan error, 1)
	go func() {
		readErr <- s.readTerminalWebSocket(ctx, conn, attachment)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readErr:
			if err != nil {
				writeCtx, cancelWrite := context.WithTimeout(context.Background(), 5*time.Second)
				_ = writeWebSocketJSON(writeCtx, conn, terminalErrorFrame(err))
				cancelWrite()
				_ = conn.Close(websocket.StatusUnsupportedData, err.Error())
			}
			return
		case frame, ok := <-attachment.Events:
			if !ok {
				return
			}
			if err := writeWebSocketJSON(ctx, conn, frame); err != nil {
				return
			}
		}
	}
}

func (s *Server) readTerminalWebSocket(ctx context.Context, conn *websocket.Conn, attachment app.TerminalAttachment) error {
	for {
		typ, payload, err := conn.Read(ctx)
		if err != nil {
			return nil
		}
		if typ != websocket.MessageText {
			return errors.New("terminal protocol requires text frames")
		}
		var msg terminalClientMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return errors.New("invalid terminal message")
		}
		switch msg.Type {
		case "input":
			data, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				return errors.New("invalid terminal input")
			}
			if err := attachment.Write(data); err != nil {
				return err
			}
		case "resize":
			if err := attachment.Resize(msg.Cols, msg.Rows); err != nil && !errors.Is(err, app.ErrTerminalExited) {
				return err
			}
		case "terminate":
			if err := attachment.Terminate(); err != nil {
				return err
			}
		default:
			return errors.New("unknown terminal message")
		}
	}
}

func (s *Server) userForWebSocketToken(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return domain.User{}, false
	}
	user, err := s.app.UserForToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, app.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return domain.User{}, false
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return domain.User{}, false
	}
	return user, true
}

func (s *Server) authorizedTerminalAccess(w http.ResponseWriter, r *http.Request, userID string, orgID string) bool {
	role, ok, err := s.authorizedOrganizationRole(r, userID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "organization not found")
		return false
	}
	if role != domain.RoleOwner && role != domain.RoleAdmin {
		writeError(w, http.StatusForbidden, "admin role required")
		return false
	}
	return true
}

func writeTerminalHTTPError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrTerminalNotFound):
		writeError(w, http.StatusNotFound, "terminal not found")
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, app.InvalidInputMessage(err))
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func terminalErrorFrame(err error) app.TerminalFrame {
	message := "terminal error"
	switch {
	case errors.Is(err, app.ErrTerminalNotFound):
		message = "terminal not found"
	case errors.Is(err, app.ErrInvalidInput):
		message = app.InvalidInputMessage(err)
	default:
		message = err.Error()
	}
	return app.TerminalFrame{Type: "error", Error: message}
}
