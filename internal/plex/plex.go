// Package plex integrates with the Plex Media Server HTTP API (read-only:
// sessions and library statistics; service control goes through systemd).
package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Status struct {
	Configured bool      `json:"configured"`
	Reachable  bool      `json:"reachable"`
	Error      string    `json:"error,omitempty"`
	Version    string    `json:"version,omitempty"`
	Sessions   []Session `json:"sessions"`
	Libraries  []Library `json:"libraries"`
}

type Session struct {
	User        string `json:"user"`
	Title       string `json:"title"`
	Grandparent string `json:"grandparent,omitempty"` // show/artist for episodes/tracks
	Type        string `json:"type"`                  // movie | episode | track
	Player      string `json:"player"`
	Product     string `json:"product"`
	State       string `json:"state"` // playing | paused | buffering
	ProgressMS  int64  `json:"progressMs"`
	DurationMS  int64  `json:"durationMs"`
	Decision    string `json:"decision"` // directplay | transcode
	BitrateKbps int    `json:"bitrateKbps,omitempty"`
}

type Library struct {
	Title string `json:"title"`
	Type  string `json:"type"` // movie | show | artist | photo
	Count int    `json:"count"`
}

type Provider interface {
	Status(ctx context.Context) Status
}

type client struct {
	baseURL string
	token   string
	http    *http.Client

	mu       sync.Mutex
	sessions []Session
	sessAt   time.Time
	version  string
	libs     []Library
	libsAt   time.Time
}

func NewClient(baseURL, token string) Provider {
	return &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 6 * time.Second},
	}
}

func (c *client) Status(ctx context.Context) Status {
	st := Status{Sessions: []Session{}, Libraries: []Library{}}
	if c.token == "" || c.baseURL == "" {
		return st
	}
	st.Configured = true

	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.sessAt) > 5*time.Second {
		sessions, version, err := c.fetchSessions(ctx)
		if err != nil {
			st.Error = err.Error()
			if c.libs != nil {
				st.Libraries = c.libs
			}
			return st
		}
		c.sessions, c.version, c.sessAt = sessions, version, time.Now()
	}
	if time.Since(c.libsAt) > 10*time.Minute {
		if libs, err := c.fetchLibraries(ctx); err == nil {
			c.libs, c.libsAt = libs, time.Now()
		}
	}
	st.Reachable = true
	st.Version = c.version
	if c.sessions != nil {
		st.Sessions = c.sessions
	}
	if c.libs != nil {
		st.Libraries = c.libs
	}
	return st
}

func (c *client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		// Strip the token if it ever appears in a transport error URL.
		return fmt.Errorf("plex unreachable: %s", strings.ReplaceAll(err.Error(), c.token, "***"))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("plex rejected the token (401)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plex returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *client) fetchSessions(ctx context.Context) ([]Session, string, error) {
	var doc struct {
		MediaContainer struct {
			Version  string `json:"version"`
			Metadata []struct {
				Title            string `json:"title"`
				GrandparentTitle string `json:"grandparentTitle"`
				Type             string `json:"type"`
				ViewOffset       int64  `json:"viewOffset"`
				Duration         int64  `json:"duration"`
				User             struct {
					Title string `json:"title"`
				} `json:"User"`
				Player struct {
					Title   string `json:"title"`
					Product string `json:"product"`
					State   string `json:"state"`
				} `json:"Player"`
				Media []struct {
					Bitrate int `json:"bitrate"`
				} `json:"Media"`
				TranscodeSession *struct{} `json:"TranscodeSession"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := c.get(ctx, "/status/sessions", &doc); err != nil {
		return nil, "", err
	}
	sessions := []Session{}
	for _, m := range doc.MediaContainer.Metadata {
		s := Session{
			User:        m.User.Title,
			Title:       m.Title,
			Grandparent: m.GrandparentTitle,
			Type:        m.Type,
			Player:      m.Player.Title,
			Product:     m.Player.Product,
			State:       m.Player.State,
			ProgressMS:  m.ViewOffset,
			DurationMS:  m.Duration,
			Decision:    "directplay",
		}
		if m.TranscodeSession != nil {
			s.Decision = "transcode"
		}
		if len(m.Media) > 0 {
			s.BitrateKbps = m.Media[0].Bitrate
		}
		sessions = append(sessions, s)
	}
	return sessions, doc.MediaContainer.Version, nil
}

func (c *client) fetchLibraries(ctx context.Context) ([]Library, error) {
	var doc struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := c.get(ctx, "/library/sections", &doc); err != nil {
		return nil, err
	}
	libs := []Library{}
	for _, d := range doc.MediaContainer.Directory {
		lib := Library{Title: d.Title, Type: d.Type}
		var count struct {
			MediaContainer struct {
				TotalSize int `json:"totalSize"`
			} `json:"MediaContainer"`
		}
		q := url.Values{"X-Plex-Container-Start": {"0"}, "X-Plex-Container-Size": {"0"}}
		if err := c.get(ctx, "/library/sections/"+url.PathEscape(d.Key)+"/all?"+q.Encode(), &count); err == nil {
			lib.Count = count.MediaContainer.TotalSize
		}
		libs = append(libs, lib)
	}
	return libs, nil
}
