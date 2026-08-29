package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/KorolevskiiDev/KWisp/internal/logstore"
)

func main() {
	app, err := logstore.New(os.Args[1:])
	if err != nil {
		slog.Error("kwisp initialization failed", "error", err)
		os.Exit(1)
	}

	if err := app.Start(); err != nil {
		slog.Error("kwisp start failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	slog.Info("shutting down", "signal", ctx.Err())

	if err := app.Stop(); err != nil {
		slog.Error("kwisp shutdown failed", "error", err)
		os.Exit(1)
	}
}
