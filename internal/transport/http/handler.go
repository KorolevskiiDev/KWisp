package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/KorolevskiiDev/KWisp/internal/domain"
	"github.com/KorolevskiiDev/KWisp/internal/service/record"
)

// Handler holds HTTP handlers for the record service.
type Handler struct {
	service    record.Service
	adminToken string
}

// NewHandler creates a new HTTP handler.
func NewHandler(service record.Service, adminToken string) *Handler {
	return &Handler{service: service, adminToken: adminToken}
}

// WriteRequest is the request body for writing records.
type WriteRequest struct {
	Records []domain.Record `json:"records"`
}

// WriteResponse is the response body for writing records.
type WriteResponse struct {
	Appended int `json:"appended"`
}

// AdminRequest is the request body for creating a stream.
type AdminRequest struct {
	Application string `json:"application"`
}

// AdminResponse is the response body for creating a stream.
type AdminResponse struct {
	Stream         string `json:"stream"`
	APIKey         string `json:"api_key"`
	PublicEndpoint string `json:"public_endpoint"`
}

// keepAlive sends periodic pings.
type keepAlive struct {
	ticker *time.Ticker
}

// newKeepAlive creates a new keep-alive pinger.
func newKeepAlive(interval int) *keepAlive {
	return &keepAlive{
		ticker: time.NewTicker(time.Duration(interval) * time.Second),
	}
}

// Ping returns a channel that receives pings.
func (k *keepAlive) Ping() <-chan time.Time {
	return k.ticker.C
}

// Stop stops the pinger.
func (k *keepAlive) Stop() {
	k.ticker.Stop()
}

// handleWrite appends a batch of records to a stream.
func (h *Handler) handleWrite(w http.ResponseWriter, r *http.Request) {
	streamName := r.PathValue("stream")
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeError(w, http.StatusUnauthorized, record.ErrInvalidKey)
		return
	}
	apiKey := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))

	var body WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(body.Records) == 0 {
		writeError(w, http.StatusBadRequest, errNoRecords)
		return
	}

	if err := h.service.WriteRecords(r.Context(), streamName, apiKey, body.Records); err != nil {
		switch err {
		case record.ErrStreamNotFound:
			writeError(w, http.StatusNotFound, err)
		case record.ErrInvalidKey:
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, WriteResponse{Appended: len(body.Records)})
}

// handleRead returns the most recent records of a stream.
func (h *Handler) handleRead(w http.ResponseWriter, r *http.Request) {
	streamName := r.PathValue("stream")
	apiKey := r.URL.Query().Get("key")
	if apiKey == "" {
		writeError(w, http.StatusUnauthorized, record.ErrInvalidKey)
		return
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			limit = n
		}
	}

	instanceID := r.URL.Query().Get("instance_id")

	var records []domain.Record
	var err error
	if instanceID != "" {
		records, err = h.service.ReadRecentFiltered(r.Context(), streamName, apiKey, instanceID, limit)
	} else {
		records, err = h.service.ReadRecent(r.Context(), streamName, apiKey, limit)
	}

	if err != nil {
		switch err {
		case record.ErrStreamNotFound:
			writeError(w, http.StatusNotFound, err)
		case record.ErrInvalidKey:
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"records": records})
}

// handleSSE streams new records of a stream as server-sent events.
func (h *Handler) handleSSE(w http.ResponseWriter, r *http.Request) {
	streamName := r.PathValue("stream")
	apiKey := r.URL.Query().Get("key")
	if apiKey == "" {
		writeError(w, http.StatusUnauthorized, record.ErrInvalidKey)
		return
	}

	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errStreamingUnsupported)
		return
	}

	ch, unsubscribe, err := h.service.Subscribe(r.Context(), streamName, apiKey)
	if err != nil {
		switch err {
		case record.ErrStreamNotFound:
			writeError(w, http.StatusNotFound, err)
		case record.ErrInvalidKey:
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fl.Flush()

	instanceID := r.URL.Query().Get("instance_id")
	keepAlive := newKeepAlive(15)

	for {
		select {
		case <-r.Context().Done():
			return
		case rec := <-ch:
			if instanceID != "" && rec.InstanceID != instanceID {
				continue
			}
			fmt.Fprint(w, "data: ")
			if err := json.NewEncoder(w).Encode(rec); err != nil {
				_ = err // ignore encode error
			}
			fmt.Fprint(w, "\n")
			fl.Flush()
		case <-keepAlive.Ping():
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// handleAdminStreams provisions a new stream.
func (h *Handler) handleAdminStreams(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if auth == "" || !bearerMatches(auth, h.adminToken) {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid admin token"))
		return
	}

	var body AdminRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Application == "" {
		writeError(w, http.StatusBadRequest, errInvalidApplication)
		return
	}

	st, err := h.service.CreateStream(r.Context(), sanitize(body.Application))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, AdminResponse{
		Stream:         st.Name,
		APIKey:         st.Key,
		PublicEndpoint: "",
	})
}

// handleHealth returns the health status.
func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
