// Package domain contains core domain types shared across layers.
package domain

// Record is a log entry as written to JSONL and served over SSE.
// JSON tags keep the wire format snake_case, matching the SDK client.
type Record struct {
	TsUnixMs    int64          `json:"ts_unix_ms"`
	Level       string         `json:"level"`
	Msg         string         `json:"msg"`
	Attrs       map[string]any `json:"attrs"`
	Application string         `json:"application"`
	InstanceID  string         `json:"instance_id"`
}
