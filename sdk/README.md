# KWeaver SDK

A Go client for the KWeaver log store service.

## Installation

```bash
go get github.com/kweaver/kweaver/sdk
```

## Usage

### Creating a Client

```go
import "github.com/kweaver/kweaver/sdk"

// Create a client pointing to your logstore instance
client := sdk.NewClient("http://localhost:8090")

// Or with options
client := sdk.NewClient(
    "http://localhost:8090",
    sdk.WithAPIKey("your-api-key"),
)
```

### Creating a Stream (Admin)

```go
// Create a stream with the admin token
stream, err := client.AdminCreateStream(context.Background(), "my-app", "admin-secret")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Stream: %s, API Key: %s\n", stream.Name, stream.APIKey)
```

### Writing Records

```go
records := []sdk.Record{
    {TsUnixMs: time.Now().UnixMilli(), Level: "info", Msg: "Application started"},
    {TsUnixMs: time.Now().UnixMilli(), Level: "error", Msg: "Something failed"},
}

err := client.WriteRecords(context.Background(), "my-app", records)
if err != nil {
    log.Fatal(err)
}
```

### Reading Recent Records

```go
// Read the last 10 records
records, err := client.ReadRecent(context.Background(), "my-app", 10)
if err != nil {
    log.Fatal(err)
}

for _, rec := range records {
    fmt.Printf("[%s] %s\n", rec.Level, rec.Msg)
}
```

### Filtering by Instance ID

```go
records, err := client.ReadRecentFiltered(context.Background(), "my-app", "instance-123", 10)
if err != nil {
    log.Fatal(err)
}
```

### Subscribing to SSE

```go
ch, cancel, err := client.Subscribe(context.Background(), "my-app", &sdk.SSEOptions{
    InstanceID: "instance-123",
})
if err != nil {
    log.Fatal(err)
}
defer cancel()

for event := range ch {
    if event.Type == "error" {
        log.Printf("Error: %s", event.Data.Msg)
        continue
    }
    fmt.Printf("[%s] %s\n", event.Data.Level, event.Data.Msg)
}
```

### Using with slog

The SDK provides a `slog.Handler` implementation for streaming logs to logstore.

```go
import (
    "log/slog"
    "github.com/kweaver/kweaver/sdk"
)

client := sdk.NewClient("http://localhost:8090", sdk.WithAPIKey("your-api-key"))

// Create a slog handler
handler := client.NewSlogHandler("my-app", &sdk.SlogHandlerOptions{
    FlushSize: 10, // Buffer 10 records before sending (default)
})
logger := slog.New(handler)

// Log normally - records are buffered and sent automatically
logger.Info("Application started", "version", "1.0.0")
logger.Error("Something failed", "error", err)

// Flush any buffered records
handler.Flush()
```

## API Reference

### Client

```go
func NewClient(baseURL string, opts ...ClientOption) *Client
func (c *Client) AdminCreateStream(ctx context.Context, application, adminToken string) (*Stream, error)
func (c *Client) WriteRecords(ctx context.Context, streamName string, records []Record) error
func (c *Client) ReadRecent(ctx context.Context, streamName string, limit int) ([]Record, error)
func (c *Client) ReadRecentFiltered(ctx context.Context, streamName, instanceID string, limit int) ([]Record, error)
func (c *Client) Subscribe(ctx context.Context, streamName string, opts *SSEOptions) (<-chan SSEEvent, func(), error)
func (c *Client) NewSlogHandler(stream string, opts *SlogHandlerOptions) slog.Handler
```

### Types

```go
type Record struct {
    TsUnixMs    int64
    Level       string
    Msg         string
    Attrs       interface{}
    Application string
    InstanceID  string
}

type Stream struct {
    Name         string
    APIKey       string
    PublicEndpoint string
}

type SSEEvent struct {
    Type string
    Data Record
}

type SSEOptions struct {
    InstanceID string
}

type SlogHandlerOptions struct {
    FlushSize int
}
```
