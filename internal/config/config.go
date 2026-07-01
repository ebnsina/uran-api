// Package config loads runtime configuration strictly from the environment.
//
// There are no defaults or fallbacks: every variable a component needs is
// required, and the loader fails fast with a single error listing everything
// missing or malformed. Configuration is split per component (api, builder,
// controller) so each process only requires the variables it actually uses.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// APIConfig is the configuration for the control-plane HTTP server.
type APIConfig struct {
	Addr                string        // URAN_ADDR
	DatabaseURL         string        // URAN_DATABASE_URL
	SessionTTL          time.Duration // URAN_SESSION_TTL
	ShutdownTimeout     time.Duration // URAN_SHUTDOWN_TIMEOUT
	Env                 string        // URAN_ENV
	GitHubWebhookSecret string        // URAN_GITHUB_WEBHOOK_SECRET
	Kubeconfig          string        // URAN_KUBECONFIG — for runtime logs/metrics
	GitHubClientID      string        // URAN_GITHUB_CLIENT_ID — OAuth App (connect + list repos)
	GitHubClientSecret  string        // URAN_GITHUB_CLIENT_SECRET
	BaseDomain          string        // URAN_BASE_DOMAIN — wildcard domain for public service URLs
}

// BuilderConfig is the configuration for the build worker.
type BuilderConfig struct {
	DatabaseURL  string // URAN_DATABASE_URL
	Env          string // URAN_ENV
	Registry     string // URAN_REGISTRY
	BuildWorkdir string // URAN_BUILD_WORKDIR
}

// ControllerConfig is the configuration for the k8s reconciler.
type ControllerConfig struct {
	DatabaseURL     string // URAN_DATABASE_URL
	Env             string // URAN_ENV
	Kubeconfig      string // URAN_KUBECONFIG
	BaseDomain      string // URAN_BASE_DOMAIN
	CertIssuer      string // URAN_CERT_ISSUER — cert-manager ClusterIssuer name
	BackupEndpoint  string // URAN_BACKUP_ENDPOINT — S3-compatible endpoint URL
	BackupBucket    string // URAN_BACKUP_BUCKET
	BackupAccessKey string // URAN_BACKUP_ACCESS_KEY
	BackupSecretKey string // URAN_BACKUP_SECRET_KEY
}

// LoadAPI loads and validates the API server configuration.
func LoadAPI() (APIConfig, error) {
	l := &loader{}
	cfg := APIConfig{
		Addr:                l.str("URAN_ADDR"),
		DatabaseURL:         l.str("URAN_DATABASE_URL"),
		SessionTTL:          l.dur("URAN_SESSION_TTL"),
		ShutdownTimeout:     l.dur("URAN_SHUTDOWN_TIMEOUT"),
		Env:                 l.str("URAN_ENV"),
		GitHubWebhookSecret: l.str("URAN_GITHUB_WEBHOOK_SECRET"),
		Kubeconfig:          l.str("URAN_KUBECONFIG"),
		GitHubClientID:      l.str("URAN_GITHUB_CLIENT_ID"),
		GitHubClientSecret:  l.str("URAN_GITHUB_CLIENT_SECRET"),
		BaseDomain:          l.str("URAN_BASE_DOMAIN"),
	}
	return cfg, l.err()
}

// LoadBuilder loads and validates the build worker configuration.
func LoadBuilder() (BuilderConfig, error) {
	l := &loader{}
	cfg := BuilderConfig{
		DatabaseURL:  l.str("URAN_DATABASE_URL"),
		Env:          l.str("URAN_ENV"),
		Registry:     l.str("URAN_REGISTRY"),
		BuildWorkdir: l.str("URAN_BUILD_WORKDIR"),
	}
	return cfg, l.err()
}

// LoadController loads and validates the controller configuration.
func LoadController() (ControllerConfig, error) {
	l := &loader{}
	cfg := ControllerConfig{
		DatabaseURL:     l.str("URAN_DATABASE_URL"),
		Env:             l.str("URAN_ENV"),
		Kubeconfig:      l.str("URAN_KUBECONFIG"),
		BaseDomain:      l.str("URAN_BASE_DOMAIN"),
		CertIssuer:      l.str("URAN_CERT_ISSUER"),
		BackupEndpoint:  l.str("URAN_BACKUP_ENDPOINT"),
		BackupBucket:    l.str("URAN_BACKUP_BUCKET"),
		BackupAccessKey: l.str("URAN_BACKUP_ACCESS_KEY"),
		BackupSecretKey: l.str("URAN_BACKUP_SECRET_KEY"),
	}
	return cfg, l.err()
}

// loader accumulates required-variable lookups and any errors encountered.
type loader struct {
	errs []string
}

func (l *loader) str(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		l.errs = append(l.errs, key+" is required")
	}
	return v
}

func (l *loader) dur(key string) time.Duration {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		l.errs = append(l.errs, key+" is required")
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s is not a valid duration: %v", key, err))
	}
	return d
}

// err returns a combined error for all problems, or nil if none.
func (l *loader) err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(l.errs, "\n  - "))
}
