// Package update self-updates the panel from GitHub releases of a single
// pinned repository: check the latest release, download the linux-amd64
// asset, verify its sha256 against the release's checksum manifest, swap
// the binary atomically (keeping the previous one as *.old), and schedule
// a systemd restart. Nothing is ever fetched from a host other than the
// GitHub API, and nothing runs without the checksum matching.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	assetName    = "control-panel-linux-amd64"
	sumsName     = "SHA256SUMS"
	maxAssetSize = 200 << 20 // hard cap on the downloaded binary
	maxSumsSize  = 1 << 20
	unitName     = "control-panel.service"
)

type Release struct {
	Tag         string `json:"tag"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"publishedAt"`
	AssetSize   int64  `json:"assetSize"`

	assetURL string
	sumsURL  string
}

type Status struct {
	Configured      bool     `json:"configured"`
	Repo            string   `json:"repo,omitempty"`
	Current         string   `json:"current"`
	Latest          *Release `json:"latest,omitempty"`
	UpdateAvailable bool     `json:"updateAvailable"`
	Error           string   `json:"error,omitempty"`
}

type Provider interface {
	// Status reports the running version and the latest release. Results are
	// cached briefly; force bypasses the cache.
	Status(ctx context.Context, force bool) Status
	// Apply downloads, verifies, and installs the release with the given tag,
	// then schedules a restart of the panel's systemd unit.
	Apply(ctx context.Context, tag string) error
}

type Client struct {
	repo    string
	token   string
	current string
	apiBase string // overridable in tests
	http    *http.Client

	mu       sync.Mutex
	cached   Status
	cachedAt time.Time
	applying bool
}

func NewClient(repo, token, current string) *Client {
	return &Client{
		repo:    repo,
		token:   token,
		current: current,
		apiBase: "https://api.github.com",
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *Client) Status(ctx context.Context, force bool) Status {
	if c.repo == "" {
		return Status{Current: c.current}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && time.Since(c.cachedAt) < 5*time.Minute {
		return c.cached
	}
	st := Status{Configured: true, Repo: c.repo, Current: c.current}
	rel, err := c.fetchRelease(ctx, "latest")
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Latest = rel
		st.UpdateAvailable = normalize(rel.Tag) != normalize(c.current)
	}
	c.cached, c.cachedAt = st, time.Now()
	return st
}

// normalize makes tags and build versions comparable: "v0.1.0" == "0.1.0".
func normalize(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func (c *Client) Apply(ctx context.Context, tag string) error {
	if c.repo == "" {
		return fmt.Errorf("updates are not configured (set update.repo)")
	}
	c.mu.Lock()
	if c.applying {
		c.mu.Unlock()
		return fmt.Errorf("an update is already in progress")
	}
	c.applying = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.applying = false
		c.cachedAt = time.Time{} // status is stale after an install either way
		c.mu.Unlock()
	}()

	// Re-fetch by tag so we install exactly what the user confirmed, not
	// whatever "latest" has become since.
	rel, err := c.fetchRelease(ctx, "tags/"+tag)
	if err != nil {
		return err
	}

	wantSum, err := c.fetchChecksum(ctx, rel)
	if err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fmt.Errorf("resolve running binary: %w", err)
	}

	// Download next to the target so the final rename is atomic.
	tmp := filepath.Join(filepath.Dir(exe), ".control-panel.update")
	if err := c.downloadVerified(ctx, rel.assetURL, tmp, wantSum); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Swap, keeping the old binary for manual rollback.
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("set aside current binary: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // roll back
		_ = os.Remove(tmp)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Transient timer restarts the unit after this response has flushed.
	out, err := exec.CommandContext(ctx, "systemd-run", "--collect",
		"--on-active=2s", "--unit=control-panel-update-restart",
		"systemctl", "restart", unitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s installed, but scheduling the restart failed (%s); "+
			"restart manually with: systemctl restart %s",
			rel.Tag, strings.TrimSpace(string(out)), unitName)
	}
	return nil
}

func (c *Client) fetchRelease(ctx context.Context, ref string) (*Release, error) {
	var doc struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := c.getJSON(ctx, c.apiBase+"/repos/"+c.repo+"/releases/"+ref, &doc); err != nil {
		return nil, err
	}
	rel := &Release{Tag: doc.TagName, Notes: doc.Body, PublishedAt: doc.PublishedAt}
	for _, a := range doc.Assets {
		switch a.Name {
		case assetName:
			rel.assetURL, rel.AssetSize = a.URL, a.Size
		case sumsName:
			rel.sumsURL = a.URL
		}
	}
	if rel.assetURL == "" || rel.sumsURL == "" {
		return nil, fmt.Errorf("release %s is missing %s or %s (CI still running?)",
			rel.Tag, assetName, sumsName)
	}
	// Assets must live on the same pinned API host we queried.
	for _, u := range []string{rel.assetURL, rel.sumsURL} {
		if !strings.HasPrefix(u, c.apiBase+"/") {
			return nil, fmt.Errorf("release asset URL %q is not on %s", u, c.apiBase)
		}
	}
	return rel, nil
}

func (c *Client) fetchChecksum(ctx context.Context, rel *Release) (string, error) {
	body, err := c.getAsset(ctx, rel.sumsURL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", sumsName, err)
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, maxSumsSize))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", sumsName, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		// "hash  name" — sha256sum writes the name with an optional * prefix.
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			if len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("malformed checksum for %s", assetName)
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s has no entry for %s", sumsName, assetName)
}

func (c *Client) downloadVerified(ctx context.Context, url, dst, wantSum string) error {
	body, err := c.getAsset(ctx, url)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetName, err)
	}
	defer body.Close()

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(body, maxAssetSize+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if n > maxAssetSize {
		return fmt.Errorf("asset exceeds the %d MB limit", maxAssetSize>>20)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSum {
		return fmt.Errorf("checksum mismatch: released binary is %s…, downloaded %s…",
			wantSum[:12], got[:12])
	}
	// Belt and braces: what we are about to install must at least be an ELF.
	head := make([]byte, 4)
	rf, err := os.Open(dst)
	if err == nil {
		_, _ = io.ReadFull(rf, head)
		rf.Close()
	}
	if string(head) != "\x7fELF" {
		return fmt.Errorf("downloaded file is not a linux executable")
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	resp, err := c.get(ctx, url, "application/vnd.github+json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("no release found (repo %s: missing, private without a "+
			"valid update.token, or no releases yet)", c.repo)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github returned %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// getAsset opens an asset download; the caller must close the body.
func (c *Client) getAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	resp, err := c.get(ctx, url, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("github returned %s", resp.Status)
	}
	return resp.Body, nil
}

func (c *Client) get(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		// net/http drops Authorization on the cross-host redirect to the
		// asset CDN, which is exactly what we want.
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.http.Do(req)
}
