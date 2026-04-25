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

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me", s.handleMe)
			r.Get("/organizations", s.handleOrganizations)
			r.Get("/organizations/{orgID}/channels", s.handleChannels)
			r.Get("/conversations/{type}/{id}/messages", s.handleListMessages)
			r.Post("/conversations/{type}/{id}/messages", s.handleSendMessage)
		})
	})

	return r
}
