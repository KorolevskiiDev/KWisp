package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a client for the logstore service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// ClientOption configures a client.
type ClientOption func(*Client)

// WithAPIKey sets the API key for all requests.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithHTTPClient sets the HTTP client to use.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// NewClient creates a new logstore client.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Record represents a log record.
type Record struct {
	TsUnixMs    int64       `json:"ts_unix_ms"`
	Level       string      `json:"level"`
	Msg         string      `json:"msg"`
	Attrs       interface{} `json:"attrs,omitempty"`
	Application string      `json:"application,omitempty"`
	InstanceID  string      `json:"instance_id,omitempty"`
}

// WriteRequest is the request body for writing records.
type WriteRequest struct {
	Records []Record `json:"records"`
}

// WriteResponse is the response body for writing records.
type WriteResponse struct {
	Appended int `json:"appended"`
}

// CreateStreamRequest is the request body for creating a stream.
type CreateStreamRequest struct {
	Application string `json:"application"`
}

// Stream represents a log stream.
type Stream struct {
	Name         string   `json:"stream"`
	APIKey       string   `json:"api_key"`
	PublicEndpoint string `json:"public_endpoint"`
}

// AdminCreateStream creates a new stream with the admin token.
func (c *Client) AdminCreateStream(ctx context.Context, application, adminToken string) (*Stream, error) {
	body, err := json.Marshal(CreateStreamRequest{Application: application})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/admin/streams", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin create failed: status %d", resp.StatusCode)
	}

	var out Stream
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

// WriteRecords writes records to a stream.
func (c *Client) WriteRecords(ctx context.Context, streamName string, records []Record) error {
	if len(records) == 0 {
		return nil
	}

	body, err := json.Marshal(WriteRequest{Records: records})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/streams/"+url.PathEscape(streamName)+"/logs", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("write failed: status %d", resp.StatusCode)
	}
	return nil
}

// ReadRecent reads recent records from a stream.
func (c *Client) ReadRecent(ctx context.Context, streamName string, limit int) ([]Record, error) {
	return c.ReadRecentFiltered(ctx, streamName, "", limit)
}

// ReadRecentFiltered reads recent records filtered by instanceID.
func (c *Client) ReadRecentFiltered(ctx context.Context, streamName, instanceID string, limit int) ([]Record, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if instanceID != "" {
		params.Set("instance_id", instanceID)
	}

	query := params.Encode()
	url := c.baseURL + "/api/streams/" + url.PathEscape(streamName)
	if query != "" {
		url += "?" + query
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if c.apiKey != "" {
		q := req.URL.Query()
		q.Add("key", c.apiKey)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read failed: status %d", resp.StatusCode)
	}

	var out struct {
		Records []Record `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out.Records, nil
}

// SSEEvent represents a server-sent event.
type SSEEvent struct {
	Type string
	Data Record
}

// SSEOptions configures SSE subscription.
type SSEOptions struct {
	InstanceID string
}

// Subscribe subscribes to a stream for new records.
func (c *Client) Subscribe(ctx context.Context, streamName string, opts *SSEOptions) (<-chan SSEEvent, func(), error) {
	params := url.Values{}
	if opts != nil && opts.InstanceID != "" {
		params.Set("instance_id", opts.InstanceID)
	}

	query := params.Encode()
	base := c.baseURL + "/api/streams/" + url.PathEscape(streamName) + "/events"
	var fullURL string
	if query != "" {
		fullURL = base + "?" + query
	} else {
		fullURL = base
	}
	if c.apiKey != "" {
		u, err := url.Parse(fullURL)
		if err != nil {
			return nil, nil, fmt.Errorf("parse url: %w", err)
		}
		u.RawQuery = u.Query().Encode() + "&key=" + url.QueryEscape(c.apiKey)
		fullURL = u.String()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("subscribe failed: status %d", resp.StatusCode)
	}

	ch := make(chan SSEEvent, 16)
	cancel := make(chan struct{})

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		reader := newEventReader(resp.Body)
		for {
			select {
			case <-cancel:
				return
			default:
			}

			event, err := reader.Read()
			if err != nil {
				if err != io.EOF {
					ch <- SSEEvent{Type: "error", Data: Record{Msg: err.Error()}}
				}
				return
			}

			if event.Type == "ping" {
				continue
			}

			ch <- event
		}
	}()

	return ch, func() { close(cancel) }, nil
}

// eventReader reads server-sent events from a body.
type eventReader struct {
	reader *bufio.Reader
}

// newEventReader creates a new event reader.
func newEventReader(r io.Reader) *eventReader {
	return &eventReader{
		reader: bufio.NewReader(r),
	}
}

// Read reads a single event.
func (r *eventReader) Read() (SSEEvent, error) {
	line, err := r.reader.ReadString('\n')
	if err != nil {
		return SSEEvent{}, err
	}

	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") {
		return r.Read()
	}

	if strings.HasPrefix(line, "data: ") {
		data := strings.TrimPrefix(line, "data: ")
		var rec Record
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			return SSEEvent{}, err
		}
		return SSEEvent{Type: "data", Data: rec}, nil
	}

	return SSEEvent{Type: line}, nil
}

// slogHandler implements slog.Handler for streaming logs to logstore.
type slogHandler struct {
	client    *Client
	stream    string
	buf       []Record
	flushSize int
}

// NewSlogHandler creates a slog.Handler that streams logs to logstore.
// It buffers records and flushes them when the buffer is full or on sync.
func (c *Client) NewSlogHandler(stream string, opts *SlogHandlerOptions) slog.Handler {
	if opts == nil {
		opts = &SlogHandlerOptions{}
	}
	h := &slogHandler{
		client:    c,
		stream:    stream,
		flushSize: 10,
	}
	if opts.FlushSize > 0 {
		h.flushSize = opts.FlushSize
	}
	return h
}

// SlogHandlerOptions configures the slog handler behavior.
type SlogHandlerOptions struct {
	// FlushSize is the number of records to buffer before sending.
	// Defaults to 10.
	FlushSize int
}

// Handle formats and queues log records for streaming.
func (h *slogHandler) Handle(_ context.Context, r slog.Record) error {
	rec := Record{
		TsUnixMs:    r.Time.UnixMilli(),
		Level:       r.Level.String(),
		Msg:         r.Message,
		Application: h.stream,
	}

	// Collect attributes
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	if len(attrs) > 0 {
		rec.Attrs = attrs
	}

	h.buf = append(h.buf, rec)

	if len(h.buf) >= h.flushSize {
		return h.Flush()
	}
	return nil
}

func (h *slogHandler) Flush() error {
	if len(h.buf) == 0 {
		return nil
	}
	err := h.client.WriteRecords(context.Background(), h.stream, h.buf)
	h.buf = nil
	return err
}

// Enabled returns whether the handler is enabled for the given level.
func (h *slogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// WithAttrs returns a new handler with the given attributes added.
func (h *slogHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

// WithGroup returns a new handler with the given group added.
func (h *slogHandler) WithGroup(_ string) slog.Handler {
	return h
}

