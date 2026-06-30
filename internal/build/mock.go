package build

import (
	"context"
	"fmt"
	"io"
)

// MockBuilder is a Builder for tests. It writes a marker to the log and returns
// either the requested image or a configured error, without touching Docker.
type MockBuilder struct {
	Err error // if non-nil, Build returns it after logging
}

// Build implements Builder.
func (m *MockBuilder) Build(ctx context.Context, req Request, logs io.Writer) (Result, error) {
	fmt.Fprintf(logs, "mock build of %s@%s -> %s\n", req.RepoURL, req.Ref, req.Image)
	if m.Err != nil {
		return Result{}, m.Err
	}
	return Result{Image: req.Image}, nil
}

var _ Builder = (*MockBuilder)(nil)
