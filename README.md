# KWeaver - Lightweight Real-time Log Streaming and Store

A lightweight, real-time external log streaming and store service - not a powerful query engine, but for fast, real-time log streaming.

## Features

- Real-time log streaming via Server-Sent Events (SSE)
- JSONL-based persistence with ring buffer caching
- Stream-based organization with API key authentication
- Admin API for stream provisioning
- Lightweight Go client SDK

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ cmd/logstore (thin entry point)                             │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ internal/bootstrap/ (dependency wiring)                     │
└─────────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
┌────────────────┐ ┌─────────────────┐ ┌──────────────────┐
│ transport/http │ │ service/record  │ │ repository/jsonl │
│   - Handlers   │ │   - Business    │ │   - JSONL        │
│   - DTOs       │ │   - Logic       │ │   - Ring Buffer  │
└────────────────┘ └─────────────────┘ └──────────────────┘
        │                   │                   │
        ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────────────┐
│ internal/domain/ (shared types)                             │
│   - Record struct                                           │
└─────────────────────────────────────────────────────────────┘
```

## Installation

### Running the Server

```bash
go run ./cmd/logstore -config config.yaml
```

### Using the Go SDK

```bash
go get github.com/kweaver/kweaver/sdk
```

## API Endpoints

### Admin

- `POST /admin/streams` - Create a new stream (requires admin token)

### Stream Operations

- `POST /api/streams/{stream}/logs` - Write records (requires API key)
- `GET /api/streams/{stream}` - Read recent records (requires API key)
- `GET /api/streams/{stream}/events` - SSE stream (requires API key)

## Configuration

```yaml
server:
  address: ":8090"
storage:
  dir: "./data"
  capacity: 1000
admin_token: "admin-secret"
public_endpoint: "http://localhost:8090"
cors:
  origins: ["*"]
```

## Example Usage

### Go SDK

```go
import "github.com/kweaver/kweaver/sdk"

client := sdk.NewClient("http://localhost:8090")

// Create stream (admin)
stream, _ := client.AdminCreateStream(ctx, "my-app", "admin-secret")

// Write records
client.WriteRecords(ctx, stream.Name, []sdk.Record{
    {Level: "info", Msg: "Hello world"},
})

// Read recent
records, _ := client.ReadRecent(ctx, stream.Name, 10)

// Subscribe to SSE
ch, _, _ := client.Subscribe(ctx, stream.Name, nil)
for event := range ch {
    fmt.Printf("[%s] %s\n", event.Data.Level, event.Data.Msg)
}
```

### HTTP (curl)

```bash
# Create stream
curl -X POST http://localhost:8090/admin/streams \
  -H "Authorization: Bearer admin-secret" \
  -H "Content-Type: application/json" \
  -d '{"application": "my-app"}'

# Write records
curl -X POST http://localhost:8090/api/streams/my-app/logs \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"records":[{"level":"info","msg":"Hello"}]}'

# Read recent
curl "http://localhost:8090/api/streams/my-app?key=your-api-key"
```

## Project Structure

- `cmd/server/` - Main entry point
- `internal/bootstrap/` - Dependency wiring
- `internal/domain/` - Core types
- `internal/repository/jsonl/` - JSONL persistence
- `internal/service/record/` - Business logic
- `internal/transport/http/` - HTTP handlers
- `sdk/` - Go client SDK

## License

MIT
