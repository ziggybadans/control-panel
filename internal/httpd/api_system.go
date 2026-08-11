package httpd

import (
	"net/http"
	"time"
)

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"info": s.SysInfo,
		"now":  time.Now().UnixMilli(),
	})
}

func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Sampler.History())
}

// handleEvents is the main SSE stream: metrics (1/s), service states,
// minecraft states, and job transitions.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sse, ok := newSSE(w, r)
	if !ok {
		return
	}
	ch, cancel := s.Bus.Subscribe(128)
	defer cancel()

	// Initial state so the client never renders empty panels.
	_ = sse.send("mc", s.MC.List())
	if svcs, err := s.Services.List(r.Context()); err == nil {
		_ = sse.send("services", svcs)
	}
	_ = sse.send("jobs", s.Jobs.List())

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if err := sse.send(ev.Name, ev.Data); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := sse.comment(); err != nil {
				return
			}
		}
	}
}
