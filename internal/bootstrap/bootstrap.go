package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/KorolevskiiDev/KWisp/internal/repository/jsonl"
	"github.com/KorolevskiiDev/KWisp/internal/service/record"
	httptransport "github.com/KorolevskiiDev/KWisp/internal/transport/http"
)

// App owns the logstore service with all dependencies wired.
type App struct {
	cfg      *Config
	repo     *jsonl.StreamRepository
	service  record.Service
	Server   *http.Server
	Router   *http.ServeMux
}

// New creates and wires all dependencies.
func New(args []string) (*App, error) {
	cfg, err := Load(parseConfigPath(args))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	repo, err := jsonl.NewRepository(cfg.Storage.Dir, cfg.Storage.Capacity)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	service := record.NewService(repo)

	mux := httptransport.NewRouter(service, cfg.AdminToken)
	httpSrv := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: withCORS(cfg.CORS.Origins, withLogging(mux)),
	}

	return &App{
		cfg:       cfg,
		repo:      repo,
		service:   service,
		Server: httpSrv,
		Router: mux,
	}, nil
}

// Start launches the HTTP server in the background.
func (a *App) Start() error {
	go func() {
		fmt.Printf("logstore listening on %s\n", a.Server.Addr)
		if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("logstore server error: %v\n", err)
		}
	}()
	return nil
}

// Stop gracefully shuts the HTTP server down.
func (a *App) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Server.Shutdown(ctx); err != nil {
		return fmt.Errorf("logstore shutdown: %w", err)
	}
	fmt.Println("logstore stopped")
	return nil
}

// withLogging logs one line per request.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		fmt.Printf("http: %s %s %d %v\n", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// withCORS allows the dashboard to call the read and SSE endpoints.
func withCORS(origins []string, next http.Handler) http.Handler {
	wildcard := false
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		if o == "*" {
			wildcard = true
			continue
		}
		allowed[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			_, ok := allowed[origin]
			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// parseConfigPath extracts the -config flag from the command line.
func parseConfigPath(args []string) string {
	fs := flag.NewFlagSet("logstore", flag.ContinueOnError)
	configPath := fs.String("config", "config-logstore.yaml", "Path to configuration file")
	_ = fs.Parse(args)
	return *configPath
}
