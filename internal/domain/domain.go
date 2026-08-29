// Package domain contains core domain types shared across layers.
package domain

// Record is a log entry as written to JSONL and served over SSE.
type Record struct {
	TsUnixMs    int64
	Level       string
	Msg         string
	Attrs       map[string]any
	Application string
	InstanceID  string
}
