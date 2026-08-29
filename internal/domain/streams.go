package domain

import (
	"errors"
)

// ErrStreamNotFound is returned when a stream does not exist.
var ErrStreamNotFound = errors.New("stream not found")

// ErrInvalidName is returned when a stream name is invalid.
var ErrInvalidName = errors.New("invalid stream name")

// ErrInternal is returned for internal errors.
var ErrInternal = errors.New("internal error")

// ErrStreamExists is returned when a stream already exists.
var ErrStreamExists = errors.New("stream exists")

// ErrInvalidKey is returned when an API key is invalid.
var ErrInvalidKey = errors.New("invalid API key")

// StreamRepository defines the interface for stream persistence.
type StreamRepository interface {
	// Get returns a stream by name, or ErrStreamNotFound if it does not exist.
	Get(name string) (*Stream, error)

	// GetOrCreate returns a stream by name, creating it if it does not exist.
	GetOrCreate(name string) (*Stream, error)

	// Delete removes a stream by name.
	Delete(name string) error
}

// Stream represents a log stream with its metadata and persistence.
type Stream struct {
	Name  string
	Key   string
	Capacity int
}
