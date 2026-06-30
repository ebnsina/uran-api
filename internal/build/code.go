package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CodeBuilder builds an image from source: if the repository has a Dockerfile
// it is built directly with `docker build`; otherwise Nixpacks auto-detects the
// stack. The result is pushed with the Docker CLI. `nixpacks` and `docker` must
// be on PATH and a Docker daemon must be reachable.
type CodeBuilder struct {
	// workdir is the base scratch directory under which per-build clone
	// directories are created. It must already exist.
	workdir string
}

// NewCodeBuilder returns a builder that uses workdir for scratch space.
func NewCodeBuilder(workdir string) *CodeBuilder {
	return &CodeBuilder{workdir: workdir}
}

// Build clones the repo, builds it (Dockerfile or Nixpacks), and pushes it.
func (b *CodeBuilder) Build(ctx context.Context, req Request, logs io.Writer) (Result, error) {
	dir, err := os.MkdirTemp(b.workdir, "build-")
	if err != nil {
		return Result{}, fmt.Errorf("create build dir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := cloneRepo(ctx, req.RepoURL, req.Ref, dir, logs); err != nil {
		return Result{}, err
	}

	if hasDockerfile(dir) {
		fmt.Fprintln(logs, "detected Dockerfile — building with docker build")
		if err := runStream(ctx, logs, dir, "docker", "build", "-t", req.Image, "."); err != nil {
			return Result{}, err
		}
	} else {
		fmt.Fprintln(logs, "no Dockerfile — building with Nixpacks")
		// --cache-key keys the build cache to this service+target so repeated
		// deploys reuse layers.
		if err := runStream(ctx, logs, "", "nixpacks", "build", dir,
			"--name", req.Image,
			"--cache-key", req.Image,
		); err != nil {
			return Result{}, err
		}
	}

	if err := runStream(ctx, logs, "", "docker", "push", req.Image); err != nil {
		return Result{}, err
	}
	return Result{Image: req.Image}, nil
}

func hasDockerfile(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "Dockerfile"))
	return err == nil && !info.IsDir()
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

var _ Builder = (*CodeBuilder)(nil)
