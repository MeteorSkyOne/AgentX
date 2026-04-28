package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/eventbus"
)

type Server struct {
	app *app.App
	bus *eventbus.Bus
}

func NewRouter(a *app.App, bus *eventbus.Bus) http.Handler {
	s := &Server{app: a, bus: bus}

	r := chi.NewRouter()
	r.Use(requestLogMiddleware)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Get("/auth/status", s.handleAuthStatus)
		r.Post("/auth/setup", s.handleSetupAdmin)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/logout", s.handleLogout)
		r.Get("/ws", s.handleWebSocket)

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me", s.handleMe)
			r.Get("/me/preferences", s.handleUserPreferences)
			r.Put("/me/preferences", s.handleUpdateUserPreferences)
			r.Get("/organizations", s.handleOrganizations)
			r.Get("/organizations/{orgID}/channels", s.handleChannels)
			r.Get("/organizations/{orgID}/notification-settings", s.handleNotificationSettings)
			r.Put("/organizations/{orgID}/notification-settings", s.handleUpdateNotificationSettings)
			r.Post("/organizations/{orgID}/notification-settings/test", s.handleTestNotificationSettings)
			r.Get("/organizations/{orgID}/projects", s.handleProjects)
			r.Post("/organizations/{orgID}/projects", s.handleCreateProject)
			r.Get("/organizations/{orgID}/agents", s.handleAgents)
			r.Post("/organizations/{orgID}/agents", s.handleCreateAgent)
			r.Patch("/projects/{projectID}", s.handleUpdateProject)
			r.Delete("/projects/{projectID}", s.handleDeleteProject)
			r.Get("/projects/{projectID}/channels", s.handleProjectChannels)
			r.Get("/projects/{projectID}/metrics", s.handleProjectMetrics)
			r.Post("/projects/{projectID}/channels", s.handleCreateChannel)
			r.Patch("/channels/{channelID}", s.handleUpdateChannel)
			r.Delete("/channels/{channelID}", s.handleArchiveChannel)
			r.Get("/channels/{channelID}/threads", s.handleThreads)
			r.Get("/channels/{channelID}/metrics", s.handleChannelMetrics)
			r.Post("/channels/{channelID}/threads", s.handleCreateThread)
			r.Get("/channels/{channelID}/agents", s.handleChannelAgents)
			r.Put("/channels/{channelID}/agents", s.handleSetChannelAgents)
			r.Patch("/threads/{threadID}", s.handleUpdateThread)
			r.Delete("/threads/{threadID}", s.handleArchiveThread)
			r.Get("/agents/{agentID}/limits", s.handleAgentLimits)
			r.Get("/agents/{agentID}/channels", s.handleAgentChannels)
			r.Patch("/agents/{agentID}", s.handleUpdateAgent)
			r.Delete("/agents/{agentID}", s.handleDeleteAgent)
			r.Get("/workspaces/{workspaceID}", s.handleWorkspace)
			r.Get("/workspaces/{workspaceID}/tree", s.handleWorkspaceTree)
			r.Get("/workspaces/{workspaceID}/files", s.handleWorkspaceFile)
			r.Put("/workspaces/{workspaceID}/files", s.handlePutWorkspaceFile)
			r.Delete("/workspaces/{workspaceID}/files", s.handleDeleteWorkspaceFile)
			r.Get("/conversations/{type}/{id}/messages", s.handleListMessages)
			r.Get("/conversations/{type}/{id}/context", s.handleConversationContext)
			r.Get("/conversations/{type}/{id}/skills", s.handleConversationSkills)
			r.Get("/conversations/{type}/{id}/metrics", s.handleConversationMetrics)
			r.Post("/conversations/{type}/{id}/messages", s.handleSendMessage)
			r.Get("/attachments/{attachmentID}/content", s.handleAttachmentContent)
			r.Patch("/messages/{messageID}", s.handleUpdateMessage)
			r.Delete("/messages/{messageID}", s.handleDeleteMessage)
		})
	})

	return r
}

func requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		}
		if status >= http.StatusInternalServerError {
			slog.Error("http request failed", attrs...)
			return
		}
		slog.Info("http request completed", attrs...)
	})
}
