package build

import (
	"context"
	"fmt"
	"io"
	"os"
)

// NixpacksBuilder builds images with Nixpacks (zero-config detection) and
// publishes them with the Docker CLI. Both `nixpacks` and `docker` must be on
// PATH and a Docker daemon must be reachable.
type NixpacksBuilder struct {
	// workdir is the base scratch directory under which per-build clone
	// directories are created. It must already exist.
	workdir string
}

// NewNixpacksBuilder returns a builder that uses workdir for scratch space.
func NewNixpacksBuilder(workdir string) *NixpacksBuilder {
	return &NixpacksBuilder{workdir: workdir}
}

// Build clones the repo, builds an image with Nixpacks, and pushes it.
func (b *NixpacksBuilder) Build(ctx context.Context, req Request, logs io.Writer) (Result, error) {
	dir, err := os.MkdirTemp(b.workdir, "build-")
	if err != nil {
		return Result{}, fmt.Errorf("create build dir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := cloneRepo(ctx, req.RepoURL, req.Ref, dir, logs); err != nil {
		return Result{}, err
	}

	// Nixpacks detects the stack and builds a Docker image tagged req.Image.
	// --cache-key keys the build cache to this service+target so repeated
	// deploys reuse layers.
	if err := runStream(ctx, logs, "", "nixpacks", "build", dir,
		"--name", req.Image,
		"--cache-key", req.Image,
	); err != nil {
		return Result{}, err
	}

	if err := runStream(ctx, logs, "", "docker", "push", req.Image); err != nil {
		return Result{}, err
	}

	return Result{Image: req.Image}, nil
}

// cloneRepo clones repoURL into dir and optionally checks out ref.
func cloneRepo(ctx context.Context, repoURL, ref, dir string, logs io.Writer) error {
	if err := runStream(ctx, logs, "", "git", "clone", repoURL, dir); err != nil {
		return err
	}
	if ref != "" {
		if err := runStream(ctx, logs, dir, "git", "checkout", ref); err != nil {
			return err
		}
	}
	return nil
}

// ensure NixpacksBuilder satisfies Builder.
var _ Builder = (*NixpacksBuilder)(nil)
