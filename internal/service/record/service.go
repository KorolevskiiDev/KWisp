package record

import (
	"context"

	"github.com/KorolevskiiDev/KWisp/internal/domain"
	"github.com/KorolevskiiDev/KWisp/internal/repository/jsonl"
)

// ErrStreamNotFound is returned when a stream does not exist.
var ErrStreamNotFound = domain.ErrStreamNotFound

// ErrInvalidKey is returned when an API key is invalid.
var ErrInvalidKey = domain.ErrInvalidKey

// ErrInternal is returned for internal errors.
var ErrInternal = domain.ErrInternal

// Service defines the record service interface.
type Service interface {
	// WriteRecords writes records to a stream.
	WriteRecords(ctx context.Context, streamName string, apiKey string, records []domain.Record) error

	// ReadRecent reads recent records from a stream.
	ReadRecent(ctx context.Context, streamName string, apiKey string, limit int) ([]domain.Record, error)

	// ReadRecentFiltered reads recent records filtered by instanceID.
	ReadRecentFiltered(ctx context.Context, streamName string, apiKey string, instanceID string, limit int) ([]domain.Record, error)

	// Subscribe subscribes to a stream for new records.
	Subscribe(ctx context.Context, streamName string, apiKey string) (chan domain.Record, func(), error)

	// CreateStream creates a new stream with a generated API key.
	CreateStream(ctx context.Context, application string) (*domain.Stream, error)
}

type service struct {
	repo *jsonl.StreamRepository
}

// NewService creates a new record service.
func NewService(repo *jsonl.StreamRepository) Service {
	return &service{repo: repo}
}

func (s *service) WriteRecords(_ context.Context, streamName string, apiKey string, records []domain.Record) error {
	st, err := s.repo.Get(streamName)
	if err != nil {
		return err
	}

	if st.Key != apiKey {
		return ErrInvalidKey
	}

	return s.repo.Append(st, records)
}

func (s *service) ReadRecent(_ context.Context, streamName string, apiKey string, limit int) ([]domain.Record, error) {
	st, err := s.repo.Get(streamName)
	if err != nil {
		return nil, err
	}

	if st.Key != apiKey {
		return nil, ErrInvalidKey
	}

	return s.repo.Recent(st, limit)
}

func (s *service) ReadRecentFiltered(_ context.Context, streamName string, apiKey string, instanceID string, limit int) ([]domain.Record, error) {
	st, err := s.repo.Get(streamName)
	if err != nil {
		return nil, err
	}

	if st.Key != apiKey {
		return nil, ErrInvalidKey
	}

	return s.repo.RecentFiltered(st, instanceID, limit)
}

func (s *service) Subscribe(_ context.Context, streamName string, apiKey string) (chan domain.Record, func(), error) {
	st, err := s.repo.Get(streamName)
	if err != nil {
		return nil, nil, err
	}

	if st.Key != apiKey {
		return nil, nil, ErrInvalidKey
	}

	return s.repo.Subscribe(st)
}

func (s *service) CreateStream(_ context.Context, application string) (*domain.Stream, error) {
	st, err := s.repo.GetOrCreate(application)
	if err != nil {
		return nil, err
	}

	return st, nil
}
