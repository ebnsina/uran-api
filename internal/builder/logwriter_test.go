package builder

import (
	"context"
	"strings"
	"testing"
)

// fakeLogStore captures appended log chunks in memory.
type fakeLogStore struct {
	buf strings.Builder
}

func (f *fakeLogStore) AppendBuildLog(_ context.Context, _ int64, chunk string) error {
	f.buf.WriteString(chunk)
	return nil
}

func TestDBLogWriter(t *testing.T) {
	fs := &fakeLogStore{}
	w := newDBLogWriter(context.Background(), fs, 1)

	n, err := w.Write([]byte("hello "))
	if err != nil || n != 6 {
		t.Fatalf("Write 1: n=%d err=%v", n, err)
	}
	if _, err := w.Write([]byte("world")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	if got := fs.buf.String(); got != "hello world" {
		t.Errorf("accumulated log = %q, want %q", got, "hello world")
	}
}
