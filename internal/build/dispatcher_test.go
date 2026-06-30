package build

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ebnsina/uran-api/internal/svctype"
)

func TestDispatcherSelectsByType(t *testing.T) {
	nix := &MockBuilder{Err: errors.New("nixpacks")}
	static := &MockBuilder{Err: errors.New("static")}
	d := NewDispatcher(nix, static)

	cases := map[string]string{
		svctype.Web:    "nixpacks",
		svctype.Worker: "nixpacks",
		svctype.Cron:   "nixpacks",
		svctype.Static: "static",
	}
	for typ, want := range cases {
		_, err := d.Build(context.Background(), Request{Type: typ}, io.Discard)
		if err == nil || err.Error() != want {
			t.Errorf("type %q routed to %v, want %q", typ, err, want)
		}
	}
}
