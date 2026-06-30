package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/ebnsina/uran-api/internal/deploy"
)

// logPollInterval is how often the SSE handler checks for new build output.
const logPollInterval = 500 * time.Millisecond

// handleDeployLogs streams a deploy's build log as Server-Sent Events, emitting
// new lines as they are persisted and closing once the build reaches a terminal
// state. The connection also ends if the client disconnects.
func (s *Server) handleDeployLogs(w http.ResponseWriter, r *http.Request) {
	d, ok := s.requireDeployAccess(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()

	sent := 0 // byte offset already streamed
	for {
		b, err := s.store.BuildByDeployID(r.Context(), d.ID)
		if err != nil {
			return // deploy/build vanished or context cancelled
		}
		if len(b.Logs) > sent {
			writeSSE(w, b.Logs[sent:])
			sent = len(b.Logs)
			flusher.Flush()
		}
		if isTerminalBuild(b.Status) {
			writeSSEEvent(w, "end", b.Status)
			flusher.Flush()
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func isTerminalBuild(status string) bool {
	return status == deploy.BuildSucceeded || status == deploy.BuildFailed
}

// writeSSE emits chunk as one or more SSE data lines (one per text line).
func writeSSE(w http.ResponseWriter, chunk string) {
	for _, line := range strings.Split(strings.TrimRight(chunk, "\n"), "\n") {
		_, _ = w.Write([]byte("data: " + line + "\n"))
	}
	_, _ = w.Write([]byte("\n"))
}

// writeSSEEvent emits a named SSE event with a single data payload.
func writeSSEEvent(w http.ResponseWriter, event, data string) {
	_, _ = w.Write([]byte("event: " + event + "\ndata: " + data + "\n\n"))
}
