package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// staticPort is the port the generated nginx server listens on. It matches the
// platform's service port so the standard web pipeline routes to it.
const staticPort = 8080

// nginxDockerfile builds a tiny nginx image serving the repository's files.
// nginx is configured to listen on the platform port and fall back to
// index.html (SPA-friendly).
const nginxDockerfile = `FROM nginx:alpine
COPY . /usr/share/nginx/html
RUN printf 'server {\n  listen %d;\n  location / {\n    root /usr/share/nginx/html;\n    try_files $uri $uri/ /index.html;\n  }\n}\n' > /etc/nginx/conf.d/default.conf
`

const dockerignore = ".git\nDockerfile\n.dockerignore\n"

// StaticBuilder builds a static site into an nginx image and publishes it with
// the Docker CLI. The repository's files are served as-is (no build step yet).
type StaticBuilder struct {
	workdir string
}

// NewStaticBuilder returns a builder that uses workdir for scratch space.
func NewStaticBuilder(workdir string) *StaticBuilder {
	return &StaticBuilder{workdir: workdir}
}

// Build clones the repo, wraps its files in an nginx image, and pushes it.
func (b *StaticBuilder) Build(ctx context.Context, req Request, logs io.Writer) (Result, error) {
	dir, err := os.MkdirTemp(b.workdir, "static-")
	if err != nil {
		return Result{}, fmt.Errorf("create build dir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := cloneRepo(ctx, req.RepoURL, req.Ref, dir, logs); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(fmt.Sprintf(nginxDockerfile, staticPort)), 0o644); err != nil {
		return Result{}, fmt.Errorf("write Dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(dockerignore), 0o644); err != nil {
		return Result{}, fmt.Errorf("write .dockerignore: %w", err)
	}

	if err := runStream(ctx, logs, dir, "docker", "build", "-t", req.Image, "."); err != nil {
		return Result{}, err
	}
	if err := runStream(ctx, logs, dir, "docker", "push", req.Image); err != nil {
		return Result{}, err
	}
	return Result{Image: req.Image}, nil
}

var _ Builder = (*StaticBuilder)(nil)
