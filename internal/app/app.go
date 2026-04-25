package app

import (
	"github.com/meteorsky/agentx/internal/eventbus"
	"github.com/meteorsky/agentx/internal/store"
)

type Options struct {
	AdminToken string
	DataDir    string
}

type App struct {
	store store.Store
	bus   *eventbus.Bus
	opts  Options
}

func New(st store.Store, bus *eventbus.Bus, opts Options) *App {
	return &App{store: st, bus: bus, opts: opts}
}
