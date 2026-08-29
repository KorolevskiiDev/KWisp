package logstore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KorolevskiiDev/KWisp/internal/domain"
)

// newTestApp starts the full logstore mux on an httptest server.
func newTestApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := fmt.Sprintf(`server:
  address: ":8090"
storage:
  dir: %q
  capacity: 100
admin_token: admin-secret
public_endpoint: http://logstore.test
cors:
  origins: ["*"]
`, filepath.Join(t.TempDir(), "data"))
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	app, err := New([]string{"-config", configPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(app.Server())
	t.Cleanup(ts.Close)
	return app, ts
}

func adminCreate(t *testing.T, ts *httptest.Server) (string, string) {
	t.Helper()
	return adminCreateAuth(t, ts, "admin-secret")
}

func adminCreateAuth(t *testing.T, ts *httptest.Server, token string) (string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"application": "example-app"})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/admin/streams", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin create status = %d, want 200", resp.StatusCode)
	}

	var out struct {
		Stream         string `json:"stream"`
		APIKey         string `json:"api_key"`
		PublicEndpoint string `json:"public_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	return out.Stream, out.APIKey
}

func writeRecords(t *testing.T, ts *httptest.Server, stream, key string, records []domain.Record) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"records": records})
	req, err := http.NewRequest(http.MethodPost,
		ts.URL+"/api/streams/"+stream+"/logs", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new write request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("write status = %d, want 200", resp.StatusCode)
	}
}

func TestAdminCreateIdempotent(t *testing.T) {
	_, ts := newTestApp(t)

	stream1, key1 := adminCreate(t, ts)
	stream2, key2 := adminCreate(t, ts)

	if stream1 != "example_app" {
		t.Errorf("stream = %q, want %q (sanitized)", stream1, "example_app")
	}
	if key1 == "" {
		t.Error("api_key is empty")
	}
	if stream2 != stream1 || key2 != key1 {
		t.Errorf("repeat create returned (%q, %q), want (%q, %q)", stream2, key2, stream1, key1)
	}
}

func TestAdminUnauthorizedAndBadBody(t *testing.T) {
	_, ts := newTestApp(t)

	body, _ := json.Marshal(map[string]string{"application": "example-app"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/streams", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong admin token status = %d, want 401", resp.StatusCode)
	}

	body, _ = json.Marshal(map[string]string{})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/admin/streams", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty application status = %d, want 400", resp.StatusCode)
	}
}

func TestWriteAndRead(t *testing.T) {
	_, ts := newTestApp(t)
	stream, key := adminCreate(t, ts)

	records := []domain.Record{
		{TsUnixMs: 1, Level: "info", Msg: "first", Application: "example-app", InstanceID: "i1"},
		{TsUnixMs: 2, Level: "error", Msg: "second", Application: "example-app", InstanceID: "i1"},
	}
	writeRecords(t, ts, stream, key, records)

	// Read with the right key.
	resp, err := http.Get(ts.URL + "/api/streams/" + stream + "?key=" + key)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Records []domain.Record `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if len(out.Records) != 2 || out.Records[0].Msg != "first" || out.Records[1].Level != "error" {
		t.Errorf("records = %+v, want both in order", out.Records)
	}

	// Wrong key 401, no key  401, unknown stream  404.
	for name, url := range map[string]string{
		"wrong key":   ts.URL + "/api/streams/" + stream + "?key=nope",
		"missing key": ts.URL + "/api/streams/" + stream,
		"unknown":     ts.URL + "/api/streams/nope?key=" + key,
	} {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		resp.Body.Close()
		want := http.StatusUnauthorized
		if name == "unknown" {
			want = http.StatusNotFound
		}
		if resp.StatusCode != want {
			t.Errorf("%s: status = %d, want %d", name, resp.StatusCode, want)
		}
	}

	// Write with the wrong key  401.
	body, _ := json.Marshal(map[string]any{"records": records[:1]})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/streams/"+stream+"/logs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer nope")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("write wrong key: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("write wrong key status = %d, want 401", resp.StatusCode)
	}
}

func TestReadInstanceFilter(t *testing.T) {
	_, ts := newTestApp(t)
	stream, key := adminCreate(t, ts)

	writeRecords(t, ts, stream, key, []domain.Record{
		{TsUnixMs: 1, Level: "info", Msg: "from-i1", InstanceID: "i1"},
		{TsUnixMs: 2, Level: "info", Msg: "from-i2", InstanceID: "i2"},
	})

	resp, err := http.Get(ts.URL + "/api/streams/" + stream + "?key=" + key + "&instance_id=i1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Records []domain.Record `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if len(out.Records) != 1 || out.Records[0].Msg != "from-i1" {
		t.Errorf("filtered records = %+v, want only the i1 record", out.Records)
	}
}

func TestSSEStreamsRecords(t *testing.T) {
	_, ts := newTestApp(t)
	stream, key := adminCreate(t, ts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/api/streams/"+stream+"/events?key="+key, nil)
	if err != nil {
		t.Fatalf("new events request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open events stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type = %q, want text/event-stream", ct)
	}

	// The subscription is registered before headers flush, so a write right
	// after Do() returns must be delivered.
	writeRecords(t, ts, stream, key, []domain.Record{{TsUnixMs: 7, Level: "info", Msg: "hello-sse", Application: "example-app"}})

	reader := bufio.NewReader(resp.Body)
	dataCh := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		dataCh <- line
	}()

	select {
	case line := <-dataCh:
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("first line = %q, want a data: event", line)
		}
		var rec domain.Record
		if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(line), "data: ")), &rec); err != nil {
			t.Fatalf("event json: %v", err)
		}
		if rec.Msg != "hello-sse" || rec.TsUnixMs != 7 {
			t.Errorf("event = %+v, want the written record", rec)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}
