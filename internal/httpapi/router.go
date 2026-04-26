package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
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
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/bootstrap", s.handleBootstrap)
		r.Post("/auth/login", s.handleLogin)
		r.Get("/ws", s.handleWebSocket)

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me", s.handleMe)
			r.Get("/organizations", s.handleOrganizations)
			r.Get("/organizations/{orgID}/channels", s.handleChannels)
			r.Get("/organizations/{orgID}/projects", s.handleProjects)
			r.Post("/organizations/{orgID}/projects", s.handleCreateProject)
			r.Get("/organizations/{orgID}/agents", s.handleAgents)
			r.Post("/organizations/{orgID}/agents", s.handleCreateAgent)
			r.Patch("/projects/{projectID}", s.handleUpdateProject)
			r.Delete("/projects/{projectID}", s.handleDeleteProject)
			r.Get("/projects/{projectID}/channels", s.handleProjectChannels)
			r.Post("/projects/{projectID}/channels", s.handleCreateChannel)
			r.Patch("/channels/{channelID}", s.handleUpdateChannel)
			r.Delete("/channels/{channelID}", s.handleArchiveChannel)
			r.Get("/channels/{channelID}/threads", s.handleThreads)
			r.Post("/channels/{channelID}/threads", s.handleCreateThread)
			r.Get("/channels/{channelID}/agents", s.handleChannelAgents)
			r.Put("/channels/{channelID}/agents", s.handleSetChannelAgents)
			r.Patch("/threads/{threadID}", s.handleUpdateThread)
			r.Delete("/threads/{threadID}", s.handleArchiveThread)
			r.Patch("/agents/{agentID}", s.handleUpdateAgent)
			r.Delete("/agents/{agentID}", s.handleDisableAgent)
			r.Get("/workspaces/{workspaceID}/tree", s.handleWorkspaceTree)
			r.Get("/workspaces/{workspaceID}/files", s.handleWorkspaceFile)
			r.Put("/workspaces/{workspaceID}/files", s.handlePutWorkspaceFile)
			r.Get("/conversations/{type}/{id}/messages", s.handleListMessages)
			r.Get("/conversations/{type}/{id}/context", s.handleConversationContext)
			r.Post("/conversations/{type}/{id}/messages", s.handleSendMessage)
			r.Patch("/messages/{messageID}", s.handleUpdateMessage)
			r.Delete("/messages/{messageID}", s.handleDeleteMessage)
		})
	})

	return r
}
