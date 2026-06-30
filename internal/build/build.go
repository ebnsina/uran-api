// Package build turns a Git repository into a runnable container image.
//
// It is deliberately free of any database or HTTP dependency: a Builder takes a
// Request and an io.Writer for logs, and returns the image it produced. The
// orchestration around it (status transitions, log persistence) lives in
// package builder.
package build

import (
	"context"
	"io"
)

// Request describes a single image build.
type Request struct {
	RepoURL string // Git URL or local path to clone
	Ref     string // commit SHA or ref to check out; empty means default branch
	Image   string // fully-qualified target image, including registry host
}

// Result is the outcome of a successful build.
type Result struct {
	Image string // the image reference that was pushed
}

// Builder produces and publishes a container image from source. Implementations
// must stream human-readable progress to logs and honour ctx cancellation.
type Builder interface {
	Build(ctx context.Context, req Request, logs io.Writer) (Result, error)
}
