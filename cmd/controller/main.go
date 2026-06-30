// Command controller reconciles built deploys onto a Kubernetes cluster:
// Deployment + Service + Traefik IngressRoute, advancing the deploy to "live".
// It runs as a separate process and shares the same Postgres as the API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ebnsina/uran-api/internal/config"
	"github.com/ebnsina/uran-api/internal/controller"
	"github.com/ebnsina/uran-api/internal/k8s"
	"github.com/ebnsina/uran-api/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("controller exited with error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.LoadController()
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

	recon, err := k8s.NewReconciler(cfg.Kubeconfig, cfg.BaseDomain, cfg.CertIssuer)
	if err != nil {
		return err
	}
	reader, err := k8s.NewReader(cfg.Kubeconfig)
	if err != nil {
		return err
	}

	proc := controller.New(st, recon, reader, log)
	return proc.Run(ctx)
}
