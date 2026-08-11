package httpd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sseWriter wraps a flushable ResponseWriter for Server-Sent Events.
type sseWriter struct {
	w  http.ResponseWriter
	fl http.Flusher
}

func newSSE(w http.ResponseWriter, r *http.Request) (*sseWriter, bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// Ask clients to reconnect quickly after a panel restart.
	fmt.Fprint(w, "retry: 2000\n\n")
	fl.Flush()
	return &sseWriter{w: w, fl: fl}, true
}

func (s *sseWriter) send(event string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return err
	}
	s.fl.Flush()
	return nil
}

func (s *sseWriter) comment() error {
	if _, err := fmt.Fprint(s.w, ": keepalive\n\n"); err != nil {
		return err
	}
	s.fl.Flush()
	return nil
}

const sseHeartbeat = 25 * time.Second
