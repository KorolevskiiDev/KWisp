// Package logstore provides the main application entry point.
package logstore

import (
	"fmt"
	"net/http"

	"github.com/KorolevskiiDev/KWisp/internal/bootstrap"
)

// App is the main application entry point.
type App struct {
	boot *bootstrap.App
}

// New creates and wires the application.
func New(args []string) (*App, error) {
	boot, err := bootstrap.New(args)
	if err != nil {
		return nil, fmt.Errorf("create bootstrap: %w", err)
	}

	return &App{
		boot: boot,
	}, nil
}

// Start launches the HTTP server.
func (a *App) Start() error {
	return a.boot.Start()
}

// Stop shuts down the HTTP server.
func (a *App) Stop() error {
	return a.boot.Stop()
}

// Server returns the HTTP server handler for testing.
func (a *App) Server() http.Handler {
	return a.boot.Router
}
