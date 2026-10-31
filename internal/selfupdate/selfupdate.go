// Package selfupdate tells when a newer Reclaimed is released and replaces
// the running Reclaimed.exe with it. The new exe must match the release's
// SHA256SUMS.txt, like every game install.
package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"reclaimed/internal/install"
	"reclaimed/internal/release"
)

// Repo is where the launcher's own releases are published.
const Repo = "tman-comps/Reclaimed-Launcher"

const (
	asset    = "Reclaimed.exe"
	sumsName = "SHA256SUMS.txt"
	maxSize  = 200 << 20
)

// Info is the latest launcher release, as far as the last check knows.
type Info struct {
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	Notes     string `json:"notes,omitempty"`
	Available bool   `json:"available"`
}

// Check asks GitHub for the latest release and compares it with current.
func Check(ctx context.Context, gh *release.Client, current string) (Info, error) {
	r, err := gh.Latest(ctx, Repo)
	if err != nil {
		return Info{}, err
	}
	v := release.NormalizeVersion(r.Tag)
	return Info{
		Latest:    v,
		URL:       r.HTMLURL,
		Notes:     r.Body,
		Available: r.Asset(asset) != nil && release.Compare(v, current) > 0,
	}, nil
}

// Apply downloads the latest Reclaimed.exe into stagingDir, checks it, and
// swaps it in for exe. The running exe can be renamed but not overwritten,
// so it's moved to exe+".old" (removed by Cleanup on the next start).
func Apply(ctx context.Context, gh *release.Client, client *http.Client, ua, exe, stagingDir string, progress install.Progress) error {
	r, err := gh.Latest(ctx, Repo)
	if err != nil {
		return err
	}
	a, s := r.Asset(asset), r.Asset(sumsName)
	if a == nil || s == nil {
		return errors.New("the latest release has no " + asset + " or " + sumsName + ", so nothing was changed")
	}
	sums, err := gh.FetchSums(ctx, s)
	if err != nil {
		return err
	}
	tmp := filepath.Join(stagingDir, asset+".new")
	if err := install.Download(ctx, client, ua, a.URL, tmp, sums[asset], maxSize, progress); err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe)
		os.Remove(tmp)
		return err
	}
	return nil
}

// Cleanup removes the exe an earlier update replaced.
func Cleanup(exe string) { os.Remove(exe + ".old") }
