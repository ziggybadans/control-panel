package qbit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// cacheTTL coalesces the dashboard widget and the qBittorrent page into
	// one round of upstream calls.
	cacheTTL = 2 * time.Second
	// errTTL keeps a failing instance from being polled at the UI's rate.
	errTTL = 10 * time.Second
	// loginBackoff is how long the client waits after credentials were
	// rejected. qBittorrent bans an address for a while after a handful of
	// failed logins, so a misconfigured password must not be retried on
	// every poll.
	loginBackoff = 5 * time.Minute
	// maxTorrents caps the payload; the full count is reported separately.
	maxTorrents = 300
	// etaUnknown is qBittorrent's "infinity" ETA (100 days).
	etaUnknown = 8640000
)

// ops maps the panel's action names onto WebUI endpoints. Anything not
// listed here cannot be invoked — the panel is not a WebUI proxy.
//
// qBittorrent 5.0 renamed pause/resume to stop/start; each entry lists the
// modern path first and the legacy fallback second.
var ops = map[string][]string{
	"pause":      {"/api/v2/torrents/stop", "/api/v2/torrents/pause"},
	"resume":     {"/api/v2/torrents/start", "/api/v2/torrents/resume"},
	"recheck":    {"/api/v2/torrents/recheck"},
	"sequential": {"/api/v2/torrents/toggleSequentialDownload"},
	"firstlast":  {"/api/v2/torrents/toggleFirstLastPiecePrio"},
	"top":        {"/api/v2/torrents/topPrio"},
	"bottom":     {"/api/v2/torrents/bottomPrio"},
	"delete":     {"/api/v2/torrents/delete"},
}

// globalOps are session-wide actions (no torrent hashes).
var globalOps = map[string]string{
	"altspeed": "/api/v2/transfer/toggleSpeedLimitsMode",
	"dllimit":  "/api/v2/transfer/setDownloadLimit",
	"uplimit":  "/api/v2/transfer/setUploadLimit",
}

// ValidOp reports whether op names an allowlisted action.
func ValidOp(op string) bool {
	_, ok := ops[op]
	if !ok {
		_, ok = globalOps[op]
	}
	return ok
}

// IsGlobalOp reports whether op acts on the session rather than torrents.
func IsGlobalOp(op string) bool {
	_, ok := globalOps[op]
	return ok
}

type client struct {
	baseURL      string
	username     string
	password     string
	allowActions bool
	http         *http.Client

	mu       sync.Mutex
	loggedIn bool
	version  string
	cached   *Status
	at       time.Time
	// authErr and authUntil hold off further login attempts after the
	// credentials were rejected (see loginBackoff).
	authErr   error
	authUntil time.Time
}

// NewClient builds a provider for one qBittorrent instance. An empty
// baseURL disables the integration.
func NewClient(baseURL, username, password string, allowActions bool) Provider {
	jar, _ := cookiejar.New(nil)
	return &client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		username:     username,
		password:     password,
		allowActions: allowActions,
		http: &http.Client{
			Timeout: 8 * time.Second,
			Jar:     jar,
			// The WebUI answers some calls with redirects to /; following
			// them turns a clear error into a confusing HTML body.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *client) Configured() bool { return c.baseURL != "" }

func (c *client) Status(ctx context.Context) Status {
	if !c.Configured() {
		return Status{Torrents: []Torrent{}}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := cacheTTL
	if c.cached != nil && !c.cached.Reachable {
		ttl = errTTL
	}
	if c.cached != nil && time.Since(c.at) < ttl {
		return *c.cached
	}
	st := Status{
		Configured:   true,
		AllowActions: c.allowActions,
		URL:          c.baseURL,
		Torrents:     []Torrent{},
	}
	if err := c.fetch(ctx, &st); err != nil {
		st.Error = err.Error()
		// Cache failures too: an unreachable client must not turn every
		// poll into an 8-second hang.
		c.cached, c.at = &st, time.Now()
		return st
	}
	st.Reachable = true
	st.Version = c.version
	c.cached, c.at = &st, time.Now()
	return st
}

// fetch fills st with the session state and torrent list.
func (c *client) fetch(ctx context.Context, st *Status) error {
	if c.version == "" {
		v, err := c.text(ctx, "/api/v2/app/version")
		if err != nil {
			return err
		}
		c.version = strings.TrimSpace(v)
	}

	var main struct {
		ServerState struct {
			DLInfoSpeed       int64  `json:"dl_info_speed"`
			UPInfoSpeed       int64  `json:"up_info_speed"`
			DLInfoData        int64  `json:"dl_info_data"`
			UPInfoData        int64  `json:"up_info_data"`
			DLRateLimit       int64  `json:"dl_rate_limit"`
			UPRateLimit       int64  `json:"up_rate_limit"`
			UseAltSpeedLimits bool   `json:"use_alt_speed_limits"`
			ConnectionStatus  string `json:"connection_status"`
			DHTNodes          int64  `json:"dht_nodes"`
			FreeSpace         int64  `json:"free_space_on_disk"`
			Queueing          bool   `json:"queueing"`
		} `json:"server_state"`
	}
	if err := c.getJSON(ctx, "/api/v2/sync/maindata?rid=0", &main); err != nil {
		return err
	}
	s := main.ServerState
	st.Transfer = Transfer{
		DLSpeed:    s.DLInfoSpeed,
		UPSpeed:    s.UPInfoSpeed,
		DLData:     s.DLInfoData,
		UPData:     s.UPInfoData,
		DLLimit:    s.DLRateLimit,
		UPLimit:    s.UPRateLimit,
		AltSpeed:   s.UseAltSpeedLimits,
		Connection: s.ConnectionStatus,
		DHTNodes:   s.DHTNodes,
		FreeSpace:  s.FreeSpace,
		Queueing:   s.Queueing,
	}

	var raw []rawTorrent
	if err := c.getJSON(ctx, "/api/v2/torrents/info", &raw); err != nil {
		return err
	}
	st.Total = len(raw)
	// Busiest first: active downloads, then everything else by add time.
	sort.SliceStable(raw, func(i, j int) bool {
		if (raw[i].DLSpeed > 0) != (raw[j].DLSpeed > 0) {
			return raw[i].DLSpeed > 0
		}
		if (raw[i].Progress < 1) != (raw[j].Progress < 1) {
			return raw[i].Progress < 1
		}
		return raw[i].AddedOn > raw[j].AddedOn
	})
	if len(raw) > maxTorrents {
		raw = raw[:maxTorrents]
	}
	for _, r := range raw {
		st.Torrents = append(st.Torrents, r.convert())
	}
	return nil
}

// rawTorrent mirrors the WebUI's torrent list entry.
type rawTorrent struct {
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	State        string  `json:"state"`
	Category     string  `json:"category"`
	Tags         string  `json:"tags"`
	Size         int64   `json:"size"`
	Downloaded   int64   `json:"downloaded"`
	AmountLeft   int64   `json:"amount_left"`
	Progress     float64 `json:"progress"`
	DLSpeed      int64   `json:"dlspeed"`
	UPSpeed      int64   `json:"upspeed"`
	ETA          int64   `json:"eta"`
	Ratio        float64 `json:"ratio"`
	NumSeeds     int     `json:"num_seeds"`
	NumComplete  int     `json:"num_complete"`
	NumLeechs    int     `json:"num_leechs"`
	NumIncomply  int     `json:"num_incomplete"`
	Priority     int     `json:"priority"`
	SeqDL        bool    `json:"seq_dl"`
	FLPiecePrio  bool    `json:"f_l_piece_prio"`
	ForceStart   bool    `json:"force_start"`
	AddedOn      int64   `json:"added_on"`
	CompletionOn int64   `json:"completion_on"`
	SavePath     string  `json:"save_path"`
	ContentPath  string  `json:"content_path"`
	Availability float64 `json:"availability"`
	Tracker      string  `json:"tracker"`
}

func (r rawTorrent) convert() Torrent {
	t := Torrent{
		Hash:         strings.ToLower(r.Hash),
		Name:         r.Name,
		State:        r.State,
		Category:     r.Category,
		Tags:         r.Tags,
		SizeBytes:    r.Size,
		Downloaded:   r.Downloaded,
		LeftBytes:    r.AmountLeft,
		Progress:     r.Progress,
		DLSpeed:      r.DLSpeed,
		UPSpeed:      r.UPSpeed,
		Ratio:        r.Ratio,
		Seeds:        r.NumSeeds,
		SeedsTotal:   r.NumComplete,
		Peers:        r.NumLeechs,
		PeersTotal:   r.NumIncomply,
		Priority:     r.Priority,
		Sequential:   r.SeqDL,
		FirstLast:    r.FLPiecePrio,
		ForceStart:   r.ForceStart,
		AddedOn:      r.AddedOn,
		SavePath:     r.SavePath,
		ContentPath:  r.ContentPath,
		Availability: r.Availability,
		Tracker:      r.Tracker,
	}
	if r.ETA > 0 && r.ETA < etaUnknown {
		t.ETASec = r.ETA
	}
	if r.CompletionOn > 0 {
		t.CompletedOn = r.CompletionOn
	}
	// Unmatched torrents still get a verdict, just without a runtime.
	t.Watch = Watchability(t, 0)
	return t
}

func (c *client) Files(ctx context.Context, hash string) ([]File, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("qBittorrent is not configured")
	}
	var raw []struct {
		Name     string  `json:"name"`
		Size     int64   `json:"size"`
		Progress float64 `json:"progress"`
		Priority int     `json:"priority"`
	}
	c.mu.Lock()
	err := c.getJSON(ctx, "/api/v2/torrents/files?hash="+url.QueryEscape(hash), &raw)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(raw))
	for _, f := range raw {
		out = append(out, File{
			Name: f.Name, SizeBytes: f.Size, Progress: f.Progress, Priority: f.Priority,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SizeBytes > out[j].SizeBytes })
	return out, nil
}

func (c *client) Do(ctx context.Context, a Action) error {
	if !c.Configured() {
		return fmt.Errorf("qBittorrent is not configured")
	}
	if !c.allowActions {
		return fmt.Errorf("qBittorrent actions are disabled (set qbittorrent.allow_actions: true)")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Any action invalidates the cached snapshot.
	defer func() { c.cached = nil }()

	if path, ok := globalOps[a.Op]; ok {
		form := url.Values{}
		if a.Op == "dllimit" || a.Op == "uplimit" {
			if a.Value < 0 {
				return fmt.Errorf("limit must be 0 (unlimited) or positive")
			}
			form.Set("limit", strconv.FormatInt(a.Value, 10))
		}
		_, err := c.post(ctx, path, form)
		return err
	}

	paths, ok := ops[a.Op]
	if !ok {
		return fmt.Errorf("unknown action %q", a.Op)
	}
	if len(a.Hashes) == 0 {
		return fmt.Errorf("no torrents selected")
	}
	form := url.Values{"hashes": {strings.Join(a.Hashes, "|")}}
	if a.Op == "delete" {
		form.Set("deleteFiles", strconv.FormatBool(a.DeleteFiles))
	}
	var err error
	for i, path := range paths {
		var status int
		status, err = c.post(ctx, path, form)
		// Only a missing endpoint justifies trying the legacy name.
		if err == nil || status != http.StatusNotFound || i == len(paths)-1 {
			break
		}
	}
	return err
}

// --- transport --------------------------------------------------------------

// login exchanges the credentials for a session cookie. Instances that
// bypass authentication (localhost exemption) need no username.
func (c *client) login(ctx context.Context) error {
	if c.username == "" {
		c.loggedIn = true
		return nil
	}
	if time.Now().Before(c.authUntil) {
		return c.authErr
	}
	form := url.Values{"username": {c.username}, "password": {c.password}}
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// qBittorrent rejects cross-site requests; it compares Referer to its
	// own address.
	req.Header.Set("Referer", c.baseURL)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("unreachable: %s", c.sanitize(err.Error()))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return c.holdOff(fmt.Errorf(
			"login refused (too many failed attempts — qBittorrent temporarily bans the panel's address)"))
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("login failed: HTTP %d", resp.StatusCode)
	case !strings.Contains(string(body), "Ok."):
		return c.holdOff(fmt.Errorf("login rejected: wrong qbittorrent.username or password"))
	}
	c.loggedIn = true
	c.authErr, c.authUntil = nil, time.Time{}
	return nil
}

// holdOff stops retrying a rejected login for a while: repeated attempts
// are what get the panel's address banned by qBittorrent.
func (c *client) holdOff(err error) error {
	c.authErr, c.authUntil = err, time.Now().Add(loginBackoff)
	return err
}

// request performs one API call, logging in first (and once more on a 403,
// which is how qBittorrent reports an expired session).
func (c *client) request(ctx context.Context, method, path string, form url.Values) (*http.Response, error) {
	send := func() (*http.Response, error) {
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, err
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.Header.Set("Referer", c.baseURL)
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("unreachable: %s", c.sanitize(err.Error()))
		}
		return resp, nil
	}

	if !c.loggedIn {
		if err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	resp, err := send()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		c.loggedIn = false
		if err := c.login(ctx); err != nil {
			return nil, err
		}
		return send()
	}
	return resp, nil
}

// post returns the response status alongside any error so callers can tell
// "no such endpoint" (older qBittorrent) from a real failure.
func (c *client) post(ctx context.Context, path string, form url.Values) (int, error) {
	if form == nil {
		form = url.Values{}
	}
	resp, err := c.request(ctx, "POST", path, form)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return resp.StatusCode, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return resp.StatusCode, fmt.Errorf("qBittorrent refused the request: %s", msg)
}

func (c *client) text(ctx context.Context, path string) (string, error) {
	resp, err := c.request(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, path)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return string(b), err
}

func (c *client) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.request(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("unexpected response from %s: %s", path, err)
	}
	return nil
}

// sanitize keeps the password out of error strings (it can appear in a
// transport error's URL).
func (c *client) sanitize(msg string) string {
	if c.password != "" {
		msg = strings.ReplaceAll(msg, c.password, "***")
	}
	return msg
}
