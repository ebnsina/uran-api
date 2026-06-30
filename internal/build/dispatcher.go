package build

import (
	"context"
	"io"

	"github.com/ebnsina/uran-api/internal/svctype"
)

// Dispatcher routes a build to the right backend based on the service type:
// static sites use the nginx StaticBuilder; everything else uses Nixpacks.
type Dispatcher struct {
	nixpacks Builder
	static   Builder
}

// NewDispatcher wires the per-type build backends.
func NewDispatcher(nixpacks, static Builder) *Dispatcher {
	return &Dispatcher{nixpacks: nixpacks, static: static}
}

// Build selects the backend for req.Type and delegates.
func (d *Dispatcher) Build(ctx context.Context, req Request, logs io.Writer) (Result, error) {
	if req.Type == svctype.Static {
		return d.static.Build(ctx, req, logs)
	}
	return d.nixpacks.Build(ctx, req, logs)
}

var _ Builder = (*Dispatcher)(nil)
