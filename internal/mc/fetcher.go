package mc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// FetchJar downloads the server jar for flavor/version into destDir and
// returns its file name.
func (f *fetcher) FetchJar(ctx context.Context, flavor, version, destDir string, out func(string)) (string, error) {
	var dlURL, name string
	switch flavor {
	case "paper":
		var builds struct {
			Builds []struct {
				Build     int `json:"build"`
				Downloads struct {
					Application struct {
						Name string `json:"name"`
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
		name = last.Downloads.Application.Name
		dlURL = fmt.Sprintf("%s/builds/%d/downloads/%s", base, last.Build, url.PathEscape(name))
	case "purpur":
		name = fmt.Sprintf("purpur-%s.jar", version)
		dlURL = fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/latest/download", url.PathEscape(version))
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
					URL string `json:"url"`
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
	if err := f.download(ctx, dlURL, filepath.Join(destDir, name), out); err != nil {
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

// download streams a URL to path with coarse progress reporting.
func (f *fetcher) download(ctx context.Context, rawURL, path string, out func(string)) error {
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

