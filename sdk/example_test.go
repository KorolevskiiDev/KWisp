package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientExample(t *testing.T) {
	// Start a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/streams":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"stream":"test-app","api_key":"test-key","public_endpoint":""}`)
		case r.Method == http.MethodPost && len(r.URL.Path) > len("/api/streams/") && r.URL.Path[len("/api/streams/"):] != "/logs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"appended":1}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/streams/test-app":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"records":[{"ts_unix_ms":1,"level":"info","msg":"hello"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/streams/test-app/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			flusher.Flush()
			// Send a test event
			fmt.Fprint(w, "data: {\"ts_unix_ms\":1,\"level\":\"info\",\"msg\":\"hello\"}\n\n")
			flusher.Flush()
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Example: Create client
	client := NewClient(ts.URL, WithAPIKey("test-key"))

	// Example: Admin create stream
	ctx := context.Background()
	stream, err := client.AdminCreateStream(ctx, "test-app", "admin-secret")
	if err != nil {
		t.Fatalf("AdminCreateStream failed: %v", err)
	}
	if stream.Name != "test-app" {
		t.Errorf("stream.Name = %q, want %q", stream.Name, "test-app")
	}

	// Example: Write records
	err = client.WriteRecords(ctx, "test-app", []Record{
		{TsUnixMs: time.Now().UnixMilli(), Level: "info", Msg: "test message"},
	})
	if err != nil {
		t.Fatalf("WriteRecords failed: %v", err)
	}

	// Example: Read recent
	records, err := client.ReadRecent(ctx, "test-app", 10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 1 || records[0].Msg != "hello" {
		t.Errorf("records = %+v, want [msg=hello]", records)
	}

	// Example: Subscribe
	ch, cancel, err := client.Subscribe(ctx, "test-app", nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer cancel()

	select {
	case event := <-ch:
		if event.Type != "data" || event.Data.Msg != "hello" {
			t.Errorf("event = %+v, want data with msg=hello", event)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSlogHandler(t *testing.T) {
	var receivedRecords []Record

	// Start a test server that captures records
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/streams/myapp/logs" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Decode and store the records
			var req WriteRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			receivedRecords = append(receivedRecords, req.Records...)

			fmt.Fprint(w, `{"appended":1}`)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, WithAPIKey("test-key"))

	// Create slog handler
	handler := client.NewSlogHandler("myapp", &SlogHandlerOptions{FlushSize: 3})
	logger := slog.New(handler)

	// Log some messages
	logger.Info("first message", "key1", "value1")
	logger.Info("second message", "key2", "value2")

	// Should not be flushed yet (need 3 records)
	if len(receivedRecords) != 0 {
		t.Errorf("expected 0 records before flush, got %d", len(receivedRecords))
	}

	// Add one more to trigger flush
	logger.Info("third message", "key3", "value3")

	// Now it should be flushed
	if len(receivedRecords) != 3 {
		t.Errorf("expected 3 records after flush, got %d", len(receivedRecords))
	}

	// Verify content
	if receivedRecords[0].Msg != "first message" {
		t.Errorf("first msg = %q, want %q", receivedRecords[0].Msg, "first message")
	}
	if receivedRecords[0].Attrs != nil {
		if attrs, ok := receivedRecords[0].Attrs.(map[string]any); ok {
			if attrs["key1"] != "value1" {
				t.Errorf("attrs key1 = %v, want %q", attrs["key1"], "value1")
			}
		}
	}

	// Log fourth message - this should be buffered, not auto-flushed
	logger.Info("fourth message")

	// Flush manually via type assertion
	type flusher interface {
		Flush() error
	}
	if f, ok := handler.(flusher); ok {
		err := f.Flush()
		if err != nil {
			t.Fatalf("flush failed: %v", err)
		}
	} else {
		t.Fatal("handler does not support Flush")
	}
	if len(receivedRecords) != 4 {
		t.Errorf("expected 4 records after flush, got %d: %v", len(receivedRecords), receivedRecords)
	}
}
