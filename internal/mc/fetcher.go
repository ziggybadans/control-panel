package mc

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// fetcher downloads server jars and version lists from the official
// project APIs (PaperMC, PurpurMC, Mojang, FabricMC).
type fetcher struct {
	http *http.Client

	mu       sync.Mutex
	versions map[string][]string
	fetched  map[string]time.Time
}

func newFetcher() *fetcher {
	return &fetcher{
		http:     &http.Client{Timeout: 60 * time.Second},
		versions: map[string][]string{},
		fetched:  map[string]time.Time{},
	}
}

func (f *fetcher) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", req.URL.Host, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Versions lists available releases for a flavor, newest first, cached 1h.
func (f *fetcher) Versions(ctx context.Context, flavor string) ([]string, error) {
	if !ValidFlavors[flavor] {
		return nil, fmt.Errorf("invalid flavor %q", flavor)
	}
	f.mu.Lock()
	if v, ok := f.versions[flavor]; ok && time.Since(f.fetched[flavor]) < time.Hour {
		f.mu.Unlock()
		return v, nil
	}
	f.mu.Unlock()

	var list []string
	var err error
	switch flavor {
	case "paper":
		var doc struct {
			Versions []string `json:"versions"`
		}
		err = f.getJSON(ctx, "https://api.papermc.io/v2/projects/paper", &doc)
		list = reversed(doc.Versions)
	case "purpur":
		var doc struct {
			Versions []string `json:"versions"`
		}
		err = f.getJSON(ctx, "https://api.purpurmc.org/v2/purpur", &doc)
		list = reversed(doc.Versions)
	case "vanilla":
		var doc struct {
			Versions []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"versions"`
		}
		err = f.getJSON(ctx, "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &doc)
		for _, v := range doc.Versions {
			if v.Type == "release" {
				list = append(list, v.ID)
			}
		}
	case "fabric":
		var doc []struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		}
		err = f.getJSON(ctx, "https://meta.fabricmc.net/v2/versions/game", &doc)
		for _, v := range doc {
			if v.Stable {
				list = append(list, v.Version)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("fetch %s versions: %w", flavor, err)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no versions returned for %s", flavor)
	}
	f.mu.Lock()
	f.versions[flavor] = list
	f.fetched[flavor] = time.Now()
	f.mu.Unlock()
	return list, nil
}

// versionArgRe confines client-supplied version strings: they are spliced
// into download URLs and local file names.
var versionArgRe = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

// checksum is an expected digest for a download ("" algo = none published).
type checksum struct {
	algo string // "sha256" | "sha1" | "md5"
	hex  string
}

// FetchJar downloads the server jar for flavor/version into destDir and
// returns its file name. Downloads are verified against the checksum the
// project API publishes (Fabric publishes none).
func (f *fetcher) FetchJar(ctx context.Context, flavor, version, destDir string, out func(string)) (string, error) {
	if !versionArgRe.MatchString(version) {
		return "", fmt.Errorf("invalid version %q", version)
	}
	var dlURL, name string
	var sum checksum
	switch flavor {
	case "paper":
		var builds struct {
			Builds []struct {
				Build     int `json:"build"`
				Downloads struct {
					Application struct {
						Name   string `json:"name"`
						SHA256 string `json:"sha256"`
					} `json:"application"`
				} `json:"downloads"`
			} `json:"builds"`
		}
		base := "https://api.papermc.io/v2/projects/paper/versions/" + url.PathEscape(version)
		if err := f.getJSON(ctx, base+"/builds", &builds); err != nil {
			return "", fmt.Errorf("paper builds for %s: %w", version, err)
		}
		if len(builds.Builds) == 0 {
			return "", fmt.Errorf("no paper builds for %s", version)
		}
		last := builds.Builds[len(builds.Builds)-1]
		// Never trust an API-supplied string as a path component.
		name = filepath.Base(last.Downloads.Application.Name)
		if name == "." || name == string(filepath.Separator) || name == "" {
			return "", fmt.Errorf("paper API returned an unusable file name %q", last.Downloads.Application.Name)
		}
		dlURL = fmt.Sprintf("%s/builds/%d/downloads/%s", base, last.Build, url.PathEscape(name))
		sum = checksum{"sha256", last.Downloads.Application.SHA256}
	case "purpur":
		var build struct {
			Build string `json:"build"`
			MD5   string `json:"md5"`
		}
		base := "https://api.purpurmc.org/v2/purpur/" + url.PathEscape(version)
		if err := f.getJSON(ctx, base+"/latest", &build); err != nil {
			return "", fmt.Errorf("purpur build for %s: %w", version, err)
		}
		if build.Build == "" {
			return "", fmt.Errorf("no purpur builds for %s", version)
		}
		name = fmt.Sprintf("purpur-%s.jar", version)
		// Pin the exact build so the verified hash matches what we download.
		dlURL = base + "/" + url.PathEscape(build.Build) + "/download"
		sum = checksum{"md5", build.MD5}
	case "vanilla":
		var manifest struct {
			Versions []struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"versions"`
		}
		if err := f.getJSON(ctx, "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &manifest); err != nil {
			return "", err
		}
		metaURL := ""
		for _, v := range manifest.Versions {
			if v.ID == version {
				metaURL = v.URL
				break
			}
		}
		if metaURL == "" {
			return "", fmt.Errorf("unknown vanilla version %s", version)
		}
		var meta struct {
			Downloads struct {
				Server struct {
					URL  string `json:"url"`
					SHA1 string `json:"sha1"`
				} `json:"server"`
			} `json:"downloads"`
		}
		if err := f.getJSON(ctx, metaURL, &meta); err != nil {
			return "", err
		}
		if meta.Downloads.Server.URL == "" {
			return "", fmt.Errorf("vanilla %s has no server download", version)
		}
		name = fmt.Sprintf("minecraft_server-%s.jar", version)
		dlURL = meta.Downloads.Server.URL
		sum = checksum{"sha1", meta.Downloads.Server.SHA1}
	case "fabric":
		loaderV, installerV, err := f.fabricComponents(ctx, version)
		if err != nil {
			return "", err
		}
		name = fmt.Sprintf("fabric-server-%s.jar", version)
		dlURL = fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/%s/server/jar",
			url.PathEscape(version), url.PathEscape(loaderV), url.PathEscape(installerV))
	default:
		return "", fmt.Errorf("invalid flavor %q", flavor)
	}

	out(fmt.Sprintf("downloading %s from %s", name, hostOf(dlURL)))
	if err := f.download(ctx, dlURL, filepath.Join(destDir, name), sum, out); err != nil {
		return "", err
	}
	return name, nil
}

func (f *fetcher) fabricComponents(ctx context.Context, game string) (loader, installer string, err error) {
	var loaders []struct {
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if err := f.getJSON(ctx, "https://meta.fabricmc.net/v2/versions/loader/"+url.PathEscape(game), &loaders); err != nil {
		return "", "", fmt.Errorf("fabric loader list: %w", err)
	}
	for _, l := range loaders {
		if l.Loader.Stable {
			loader = l.Loader.Version
			break
		}
	}
	if loader == "" && len(loaders) > 0 {
		loader = loaders[0].Loader.Version
	}
	var installers []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := f.getJSON(ctx, "https://meta.fabricmc.net/v2/versions/installer", &installers); err != nil {
		return "", "", fmt.Errorf("fabric installer list: %w", err)
	}
	for _, i := range installers {
		if i.Stable {
			installer = i.Version
			break
		}
	}
	if loader == "" || installer == "" {
		return "", "", fmt.Errorf("no fabric loader/installer available for %s", game)
	}
	return loader, installer, nil
}

// download streams a URL to path with coarse progress reporting, verifying
// sum (when one is published) before the file becomes visible at path.
func (f *fetcher) download(ctx context.Context, rawURL, path string, sum checksum, out func(string)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	tmp := path + ".partial"
	w, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		w.Close()
		os.Remove(tmp)
	}()

	var hasher hash.Hash
	switch sum.algo {
	case "sha256":
		hasher = sha256.New()
	case "sha1":
		hasher = sha1.New()
	case "md5":
		hasher = md5.New()
	}
	if hasher != nil && sum.hex == "" {
		// The API contract includes a digest; treat its absence as an error
		// rather than silently skipping verification.
		return fmt.Errorf("no %s checksum published for this download", sum.algo)
	}

	total := resp.ContentLength
	var done int64
	lastPct := -10
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if hasher != nil {
				hasher.Write(buf[:n])
			}
			done += int64(n)
			if total > 0 {
				pct := int(done * 100 / total)
				if pct >= lastPct+10 {
					lastPct = pct
					out(fmt.Sprintf("%d%% (%.1f / %.1f MiB)", pct,
						float64(done)/(1<<20), float64(total)/(1<<20)))
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if hasher != nil {
		got := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(got, sum.hex) {
			return fmt.Errorf("checksum mismatch: %s of download is %s, expected %s", sum.algo, got, sum.hex)
		}
		out(sum.algo + " checksum verified")
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

func reversed(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}
