package httpd

import (
	"net/http"
	"time"
)

// handleSummary returns one compact payload for lightweight clients (the
// macOS menu bar app). It composes cached provider data, so polling it every
// few seconds is cheap.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	type poolSummary struct {
		Mount string `json:"mount"`
		Used  uint64 `json:"used"`
		Total uint64 `json:"total"`
	}
	type mcSummary struct {
		ID         string `json:"id"`
		State      string `json:"state"`
		Players    int    `json:"players"`
		MaxPlayers int    `json:"maxPlayers"`
	}
	type summary struct {
		Hostname string  `json:"hostname"`
		Mock     bool    `json:"mock"`
		Now      int64   `json:"now"`
		BootTime int64   `json:"bootTime"`
		CPU      float64 `json:"cpu"`
		MemUsed  uint64  `json:"memUsed"`
		MemTotal uint64  `json:"memTotal"`

		Pool           *poolSummary `json:"pool,omitempty"`
		DisksFlagged   int          `json:"disksFlagged"`
		DisksUnhealthy int          `json:"disksUnhealthy"`

		ServicesFailed []string    `json:"servicesFailed"`
		MC             []mcSummary `json:"mc"`
		PlexConfigured bool        `json:"plexConfigured"`
		PlexStreams    int         `json:"plexStreams"`

		// Media apps rollup (0 when none are configured).
		AppsConfigured  bool `json:"appsConfigured"`
		AppsQueue       int  `json:"appsQueue"`
		AppsIssues      int  `json:"appsIssues"` // health issues + unreachable apps
		RequestsPending int  `json:"requestsPending"`

		JobRunning string `json:"jobRunning,omitempty"` // "kind target"
	}

	out := summary{
		Hostname:       s.SysInfo.Hostname,
		Mock:           s.SysInfo.Mock,
		Now:            time.Now().UnixMilli(),
		BootTime:       s.SysInfo.BootTime,
		ServicesFailed: []string{},
		MC:             []mcSummary{},
	}

	if hist := s.Sampler.History(); len(hist) > 0 {
		last := hist[len(hist)-1]
		out.CPU = last.CPU
		out.MemUsed = last.MemUsed
		out.MemTotal = last.MemTotal
	}

	// Storage (Overview is memoized inside the provider).
	if ov, err := s.Storage.Overview(r.Context()); err == nil {
		if len(ov.Pools) > 0 {
			p := ov.Pools[0]
			out.Pool = &poolSummary{Mount: p.Mount, Used: p.Used, Total: p.Total}
		}
		for _, d := range ov.Disks {
			if d.Smart.Healthy != nil && !*d.Smart.Healthy {
				out.DisksUnhealthy++
			} else if d.Smart.Reallocated > 0 || d.Smart.PendingSectors > 0 {
				out.DisksFlagged++
			}
		}
	}

	if svcs, err := s.Services.List(r.Context()); err == nil {
		for _, svc := range svcs {
			if svc.ActiveState == "failed" {
				out.ServicesFailed = append(out.ServicesFailed, svc.Unit)
			}
		}
	}

	for _, srv := range s.MC.List() {
		out.MC = append(out.MC, mcSummary{
			ID:         srv.ID,
			State:      string(srv.State),
			Players:    len(srv.OnlinePlayers),
			MaxPlayers: srv.MaxPlayers,
		})
	}

	plexStatus := s.Plex.Status(r.Context())
	out.PlexConfigured = plexStatus.Configured
	out.PlexStreams = len(plexStatus.Sessions)

	if s.Apps.Configured() {
		out.AppsConfigured = true
		for _, app := range s.Apps.List(r.Context()) {
			out.AppsQueue += app.QueueCount
			out.AppsIssues += len(app.HealthIssues)
			if !app.Reachable {
				out.AppsIssues++
			}
			if app.Requests != nil {
				out.RequestsPending += app.Requests.Pending
			}
		}
	}

	if job := s.Jobs.Current(); job != nil {
		out.JobRunning = job.Kind + " " + job.Target
	}

	writeJSON(w, http.StatusOK, out)
}
