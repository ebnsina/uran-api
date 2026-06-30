// Command builder consumes queued deploys from the event bus and builds them
// into container images (clone -> Nixpacks -> push). It runs as a separate
// process from the API and shares the same Postgres.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ebnsina/uran-api/internal/build"
	"github.com/ebnsina/uran-api/internal/builder"
	"github.com/ebnsina/uran-api/internal/config"
	"github.com/ebnsina/uran-api/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("builder exited with error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadBuilder()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	backend := build.NewDispatcher(
		build.NewNixpacksBuilder(cfg.BuildWorkdir),
		build.NewStaticBuilder(cfg.BuildWorkdir),
	)
	proc := builder.New(st, backend, cfg.Registry, log)
	return proc.Run(ctx)
}
