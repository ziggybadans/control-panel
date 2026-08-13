package httpd

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/ziggybadans/control-panel/internal/files"
)

// startedAt approximates process start (the server is built during startup).
var startedAt = time.Now()

func fileRootCount(s *files.Service) int {
	if s == nil {
		return 0
	}
	return len(s.Roots())
}

// panelInfo describes the panel process and its configuration for the About
// card. Secrets (tokens, API keys, password hash) never appear here.
type panelInfo struct {
	GoVersion      string      `json:"goVersion"`
	Pid            int         `json:"pid"`
	StartedAt      int64       `json:"startedAt"` // unix ms
	DataDir        string      `json:"dataDir"`
	Listen         string      `json:"listen"`
	TLS            bool        `json:"tls"`
	AuthMode       string      `json:"authMode"`
	SessionHours   int         `json:"sessionHours"`
	TrustedProxies int         `json:"trustedProxies"`
	Features       featureInfo `json:"features"`
}

type featureInfo struct {
	Smart          bool   `json:"smart"`
	Snapraid       bool   `json:"snapraid"`
	Plex           bool   `json:"plex"`
	Apps           int    `json:"apps"`
	MinecraftRoot  string `json:"minecraftRoot"`
	MinecraftRunAs string `json:"minecraftRunAs,omitempty"`
	Power          bool   `json:"power"`
	UpdateRepo     string `json:"updateRepo,omitempty"`
	FanControl     bool   `json:"fanControl"`
	FileRoots      int    `json:"fileRoots"`
	Terminal       bool   `json:"terminal"`
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"info": s.SysInfo,
		"now":  time.Now().UnixMilli(),
		"panel": panelInfo{
			GoVersion:      runtime.Version(),
			Pid:            os.Getpid(),
			StartedAt:      startedAt.UnixMilli(),
			DataDir:        s.Cfg.DataDir,
			Listen:         s.Cfg.Listen,
			TLS:            s.Cfg.TLS.Cert != "",
			AuthMode:       s.Cfg.Auth.Mode,
			SessionHours:   s.Cfg.Auth.SessionHours,
			TrustedProxies: len(s.Cfg.TrustedProxies),
			Features: featureInfo{
				Smart:          s.Cfg.Storage.Smart,
				Snapraid:       s.Cfg.Storage.Snapraid.Config != "",
				Plex:           s.Cfg.Plex.Token != "",
				Apps:           len(s.Cfg.Apps),
				MinecraftRoot:  s.Cfg.Minecraft.Root,
				MinecraftRunAs: s.Cfg.Minecraft.RunAs,
				Power:          s.Cfg.Power.Allow,
				UpdateRepo:     s.Cfg.Update.Repo,
				FanControl:     s.Cfg.FanControl(),
				FileRoots:      fileRootCount(s.Files),
				Terminal:       s.Term != nil && s.Term.Enabled(),
			},
		},
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
	if s.Fans != nil {
		_ = sse.send("fans", s.Fans.LiveState())
	}
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
