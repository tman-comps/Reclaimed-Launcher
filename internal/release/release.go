// Package release reads a project's latest GitHub release and its
// SHA256SUMS.txt. No authentication is used, so requests are cached with
// ETags to stay inside GitHub's anonymous rate limit.
package release

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Asset is one file attached to a release.
type Asset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"` // "sha256:<hex>", newer GitHub API responses
}

// Release is the subset of GitHub's release object the launcher needs.
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
}

// Asset returns the named asset, or nil.
func (r *Release) Asset(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// Client fetches releases. BaseURL is overridable for tests.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	UA      string

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	etag string
	body []byte
}

// NewClient returns a client for api.github.com.
func NewClient(ua string) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		BaseURL: "https://api.github.com",
		UA:      ua,
		cache:   map[string]cached{},
	}
}

// Latest returns the latest published (non-draft, non-prerelease) release of
// owner/repo.
func (c *Client) Latest(ctx context.Context, repo string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(c.BaseURL, "/"), repo)
	body, err := c.getCached(ctx, url)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if r.Tag == "" {
		return nil, errors.New("release has no tag")
	}
	return &r, nil
}

func (c *Client) getCached(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UA)
	c.mu.Lock()
	prev, hasPrev := c.cache[url]
	c.mu.Unlock()
	if hasPrev && prev.etag != "" {
		req.Header.Set("If-None-Match", prev.etag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified && hasPrev:
		return prev.body, nil
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.cache[url] = cached{etag: resp.Header.Get("ETag"), body: body}
		c.mu.Unlock()
		return body, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("no published release found")
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			reset, _ := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)
			return nil, fmt.Errorf("GitHub rate limit reached; try again after %s", time.Unix(reset, 0).Format("15:04"))
		}
		return nil, fmt.Errorf("GitHub refused the request (%s)", resp.Status)
	default:
		return nil, fmt.Errorf("GitHub returned %s", resp.Status)
	}
}

// FetchSums downloads and parses a SHA256SUMS.txt asset.
func (c *Client) FetchSums(ctx context.Context, a *Asset) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UA)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SHA256SUMS download: %s", resp.Status)
	}
	return ParseSums(io.LimitReader(resp.Body, 1<<20))
}

// ParseSums parses sha256sum output: "<64 hex>  name" or "<64 hex> *name".
// Lines that don't match are rejected rather than skipped, so a garbled file
// can't silently drop the entry we need.
func ParseSums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("SHA256SUMS line %d is malformed", n)
		}
		sum := strings.ToLower(fields[0])
		if b, err := hex.DecodeString(sum); err != nil || len(b) != 32 {
			return nil, fmt.Errorf("SHA256SUMS line %d has no valid SHA-256", n)
		}
		name := strings.TrimPrefix(strings.TrimSpace(line[len(fields[0]):]), "*")
		name = strings.TrimPrefix(name, "./")
		out[name] = sum
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("SHA256SUMS is empty")
	}
	return out, nil
}

// Pattern picks a release asset by regexp; group 1 (if any) is the version.
type Pattern struct {
	Re      *regexp.Regexp
	Exclude *regexp.Regexp
}

// Pick returns the first asset matching the first pattern that matches
// anything, so newer naming schemes win over legacy ones.
func Pick(r *Release, patterns []Pattern) *Asset {
	for _, p := range patterns {
		for i := range r.Assets {
			name := r.Assets[i].Name
			if p.Exclude != nil && p.Exclude.MatchString(name) {
				continue
			}
			if p.Re.MatchString(name) {
				return &r.Assets[i]
			}
		}
	}
	return nil
}

// NormalizeVersion strips a leading "v" and surrounding space.
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 1 && (v[0] == 'v' || v[0] == 'V') && v[1] >= '0' && v[1] <= '9' {
		v = v[1:]
	}
	return v
}

// Compare compares dotted versions with an optional -prerelease suffix.
// It returns -1, 0 or 1. Non-numeric parts compare as strings.
func Compare(a, b string) int {
	a, b = NormalizeVersion(a), NormalizeVersion(b)
	split := func(v string) (string, string) {
		v = strings.SplitN(v, "+", 2)[0]
		if i := strings.IndexByte(v, '-'); i >= 0 {
			return v[:i], v[i+1:]
		}
		return v, ""
	}
	ac, ap := split(a)
	bc, bp := split(b)
	as, bs := strings.Split(ac, "."), strings.Split(bc, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := "0", "0"
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := cmpPart(x, y); c != 0 {
			return c
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1 // release > prerelease
	case bp == "":
		return -1
	}
	return cmpPart(ap, bp)
}

func cmpPart(x, y string) int {
	xi, ex := strconv.Atoi(x)
	yi, ey := strconv.Atoi(y)
	if ex == nil && ey == nil {
		switch {
		case xi < yi:
			return -1
		case xi > yi:
			return 1
		}
		return 0
	}
	return strings.Compare(x, y)
}
