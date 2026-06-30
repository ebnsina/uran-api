package builder

import "context"

// logStore is the subset of the store the log writer needs.
type logStore interface {
	AppendBuildLog(ctx context.Context, buildID int64, chunk string) error
}

// dbLogWriter is an io.Writer that appends every chunk to a build's persisted
// log. It is used as the log sink passed to a Builder.
type dbLogWriter struct {
	ctx     context.Context
	store   logStore
	buildID int64
}

func newDBLogWriter(ctx context.Context, store logStore, buildID int64) *dbLogWriter {
	return &dbLogWriter{ctx: ctx, store: store, buildID: buildID}
}

// Write persists p to the build log. It reports a short write as an error so
// callers (e.g. exec piping) surface persistence failures.
func (w *dbLogWriter) Write(p []byte) (int, error) {
	if err := w.store.AppendBuildLog(w.ctx, w.buildID, string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
