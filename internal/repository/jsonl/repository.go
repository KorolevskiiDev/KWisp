package jsonl

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/KorolevskiiDev/KWisp/internal/domain"
)

// StreamRepository implements domain.StreamRepository using JSONL files.
type StreamRepository struct {
	mu      sync.Mutex
	dir     string
	capacity int
	streams map[string]*stream
}

type stream struct {
	mu      sync.Mutex
	name    string
	key     string
	file    *os.File
	ring    []domain.Record
	next    int
	count   int
	subs    map[chan domain.Record]struct{}
}

// NewRepository creates a new JSONL repository.
func NewRepository(dir string, capacity int) (*StreamRepository, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	repo := &StreamRepository{
		dir:      dir,
		capacity: capacity,
		streams:  make(map[string]*stream),
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	for _, e := range entries {
		if streamName, ok := strings.CutSuffix(e.Name(), ".log"); ok {
			if _, err := repo.openStream(streamName); err != nil {
				return nil, err
			}
		}
	}

	return repo, nil
}

// Get returns a stream by name, or domain.ErrStreamNotFound if it does not exist.
func (r *StreamRepository) Get(name string) (*domain.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.streams[name]
	if !ok {
		return nil, domain.ErrStreamNotFound
	}

	return &domain.Stream{
		Name:     st.name,
		Key:      st.key,
		Capacity: r.capacity,
	}, nil
}

// GetOrCreate returns a stream by name, creating it if it does not exist.
func (r *StreamRepository) GetOrCreate(name string) (*domain.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.streams[name]
	if ok {
		return &domain.Stream{
			Name:     st.name,
			Key:      st.key,
			Capacity: r.capacity,
		}, nil
	}

	st, err := r.openStream(name)
	if err != nil {
		return nil, err
	}

	return &domain.Stream{
		Name:     st.name,
		Key:      st.key,
		Capacity: r.capacity,
	}, nil
}

// Delete removes a stream by name.
func (r *StreamRepository) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.streams[name]
	if !ok {
		return domain.ErrStreamNotFound
	}

	st.mu.Lock()
	if st.file != nil {
		st.file.Close()
	}
	st.mu.Unlock()

	delete(r.streams, name)

	keyPath := filepath.Join(r.dir, name+".key")
	logPath := filepath.Join(r.dir, name+".log")

	os.Remove(keyPath)
	os.Remove(logPath)

	return nil
}

func (r *StreamRepository) openStream(name string) (*stream, error) {
	st := &stream{
		name:     name,
		key:      "",
		file:     nil,
		ring:     make([]domain.Record, r.capacity),
		subs:     make(map[chan domain.Record]struct{}),
	}

	key, err := r.readKey(filepath.Join(r.dir, name+".key"))
	if err != nil {
		key, err = r.newKey()
		if err != nil {
			return nil, fmt.Errorf("generate api key: %w", err)
		}
		if err := os.WriteFile(filepath.Join(r.dir, name+".key"), []byte(key), 0o600); err != nil {
			return nil, fmt.Errorf("persist api key: %w", err)
		}
	}
	st.key = key

	f, err := os.OpenFile(filepath.Join(r.dir, name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	st.file = f

	if err := r.loadJSONLTail(st); err != nil {
		f.Close()
		return nil, fmt.Errorf("load log tail for %s: %w", name, err)
	}

	r.streams[name] = st
	return st, nil
}

// Append persists a record to the JSONL file and adds it to the ring buffer.
func (r *StreamRepository) Append(stream *domain.Stream, records []domain.Record) error {
	r.mu.Lock()
	st, ok := r.streams[stream.Name]
	r.mu.Unlock()

	if !ok {
		return domain.ErrStreamNotFound
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	for _, rec := range records {
		b, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshal record: %w", err)
		}
		b = append(b, '\n')
		if _, err := st.file.Write(b); err != nil {
			return fmt.Errorf("append record: %w", err)
		}

		st.ring[st.next] = rec
		st.next = (st.next + 1) % len(st.ring)
		if st.count < len(st.ring) {
			st.count++
		}

		for ch := range st.subs {
			select {
			case ch <- rec:
			default:
			}
		}
	}

	return nil
}

// Recent returns up to n of the newest records, oldest first.
func (r *StreamRepository) Recent(stream *domain.Stream, n int) ([]domain.Record, error) {
	r.mu.Lock()
	st, ok := r.streams[stream.Name]
	r.mu.Unlock()

	if !ok {
		return nil, domain.ErrStreamNotFound
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	count := st.count
	if n > 0 && n < count {
		count = n
	}

	out := make([]domain.Record, 0, count)
	start := (st.next - count + len(st.ring)) % len(st.ring)
	for i := 0; i < count; i++ {
		out = append(out, st.ring[(start+i)%len(st.ring)])
	}

	return out, nil
}

// RecentFiltered returns up to n of the newest records matching instanceID.
func (r *StreamRepository) RecentFiltered(stream *domain.Stream, instanceID string, n int) ([]domain.Record, error) {
	r.mu.Lock()
	st, ok := r.streams[stream.Name]
	r.mu.Unlock()

	if !ok {
		return nil, domain.ErrStreamNotFound
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	matched := make([]domain.Record, 0, min(n, st.count))
	for i := 1; i <= st.count; i++ {
		rec := st.ring[(st.next-i+len(st.ring))%len(st.ring)]
		if rec.InstanceID != instanceID {
			continue
		}
		matched = append(matched, rec)
		if n > 0 && len(matched) >= n {
			break
		}
	}

	out := make([]domain.Record, len(matched))
	for i, rec := range matched {
		out[len(matched)-1-i] = rec
	}

	return out, nil
}

// Subscribe registers a new subscriber channel.
func (r *StreamRepository) Subscribe(stream *domain.Stream) (chan domain.Record, func(), error) {
	r.mu.Lock()
	st, ok := r.streams[stream.Name]
	r.mu.Unlock()

	if !ok {
		return nil, nil, domain.ErrStreamNotFound
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	ch := make(chan domain.Record, 256)
	st.subs[ch] = struct{}{}

	unsubscribe := func() {
		st.mu.Lock()
		delete(st.subs, ch)
		st.mu.Unlock()
		close(ch)
	}

	return ch, unsubscribe, nil
}

// loadJSONLTail reads the JSONL file into the ring buffer.
func (r *StreamRepository) loadJSONLTail(st *stream) error {
	f, err := os.Open(st.file.Name())
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec domain.Record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		st.ring[st.next] = rec
		st.next = (st.next + 1) % len(st.ring)
		if st.count < len(st.ring) {
			st.count++
		}
	}

	return sc.Err()
}

func (r *StreamRepository) readKey(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (r *StreamRepository) newKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
