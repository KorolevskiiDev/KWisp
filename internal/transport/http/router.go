package http

import (
	"net/http"

	"github.com/KorolevskiiDev/KWisp/internal/service/record"
)

// NewRouter creates a new HTTP router with all routes wired.
func NewRouter(service record.Service, adminToken string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/streams", newHandler(service, adminToken).handleAdminStreams)
	mux.HandleFunc("POST /api/streams/{stream}/logs", newHandler(service, adminToken).handleWrite)
	mux.HandleFunc("GET /api/streams/{stream}/events", newHandler(service, adminToken).handleSSE)
	mux.HandleFunc("GET /api/streams/{stream}", newHandler(service, adminToken).handleRead)
	mux.HandleFunc("GET /healthz", newHandler(service, adminToken).handleHealth)
	return mux
}

// newHandler creates a new handler for the service.
func newHandler(service record.Service, adminToken string) *Handler {
	return &Handler{service: service, adminToken: adminToken}
}
