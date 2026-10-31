package games

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"reclaimed/internal/art"
	"reclaimed/internal/install"
	"reclaimed/internal/paths"
	"reclaimed/internal/proc"
	"reclaimed/internal/release"
	"reclaimed/internal/state"
	"reclaimed/internal/steam"
)

const maxDownload = 8 << 30

// Status is what the UI needs to draw a tile.
type Status struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Tagline   string `json:"tagline"`
	NeedsName string `json:"needsName"`

	Installed bool   `json:"installed"`
	Linked    bool   `json:"linked"` // existing install outside the launcher folder
	Dir       string `json:"dir"`
	Exe       string `json:"exe"`
	Version   string `json:"version"`
	// VersionSource: "launcher" (what we installed), "release" (matched a
	// release checksum), "file" (exe version resource) or "" (unknown).
	VersionSource string `json:"versionSource"`
	UpdatedInGame bool   `json:"updatedInGame"`

	Latest          string    `json:"latest"`
	LatestURL       string    `json:"latestUrl"`
	LatestNotes     string    `json:"latestNotes"`
	LatestPublished time.Time `json:"latestPublished"`
	LatestAsset     string    `json:"latestAsset"`
	LatestSize      int64     `json:"latestSize"`
	CheckError      string    `json:"checkError,omitempty"`
	CheckedAt       time.Time `json:"checkedAt"`
	UpdateAvailable bool      `json:"updateAvailable"`

	Running bool   `json:"running"`
	Busy    string `json:"busy,omitempty"`

	RequirementOK  bool   `json:"requirementOk"`
	RequirementMsg string `json:"requirementMsg,omitempty"`
	Blocker        string `json:"blocker,omitempty"` // prevents installing at all

	UserDataDir    string `json:"userDataDir"`
	UserDataExists bool   `json:"userDataExists"`
	SelfUpdateNote string `json:"selfUpdateNote"`

	ArtTall    string `json:"artTall,omitempty"` // cover URLs (custom or built-in)
	ArtWide    string `json:"artWide,omitempty"`
	ArtTallSrc string `json:"artTallSrc,omitempty"` // "custom" or "built-in"
	ArtWideSrc string `json:"artWideSrc,omitempty"`
}

type latestInfo struct {
	rel     *release.Release
	asset   *release.Asset
	sums    map[string]string
	version string
	err     error
	at      time.Time
}

// Manager installs, inspects and launches games.
type Manager struct {
	L     paths.Layout
	Store *state.Store
	GH    *release.Client
	HTTP  *http.Client
	UA    string
	MCC   func() steam.MCCInfo
	Defs  []*Def
	// BeforeLaunch, if set, runs right before Launch starts a game (after
	// its checks pass).
	BeforeLaunch func(id string)
	locks        map[string]*sync.Mutex
	mu           sync.Mutex
	latest       map[string]*latestInfo
	busy         map[string]string
	hashes       map[string]hashEntry
}

type hashEntry struct {
	size int64
	mod  time.Time
	sum  string
}

// hashExe hashes a file, reusing the last result while size and mtime match.
func (m *Manager) hashExe(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	m.mu.Lock()
	h, ok := m.hashes[path]
	m.mu.Unlock()
	if ok && h.size == st.Size() && h.mod.Equal(st.ModTime()) {
		return h.sum
	}
	sum, err := install.HashFile(path)
	if err != nil {
		return ""
	}
	m.mu.Lock()
	m.hashes[path] = hashEntry{st.Size(), st.ModTime(), sum}
	m.mu.Unlock()
	return sum
}

// NewManager wires a manager.
func NewManager(l paths.Layout, st *state.Store, gh *release.Client, ua string, mcc func() steam.MCCInfo) *Manager {
	m := &Manager{
		L: l, Store: st, GH: gh, UA: ua, MCC: mcc, Defs: All,
		HTTP:   &http.Client{Timeout: 0}, // large downloads; cancelled by context
		locks:  map[string]*sync.Mutex{},
		latest: map[string]*latestInfo{},
		busy:   map[string]string{},
		hashes: map[string]hashEntry{},
	}
	for _, d := range m.Defs {
		m.locks[d.ID] = &sync.Mutex{}
	}
	return m
}

func (m *Manager) def(id string) (*Def, error) {
	for _, d := range m.Defs {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, fmt.Errorf("unknown game %q", id)
}

// Dir is where the game lives: the launcher's games folder, or a linked one.
func (m *Manager) Dir(id string) string {
	if g := m.Store.Get(id); g.LinkedDir != "" {
		return g.LinkedDir
	}
	return m.L.GameDir(id)
}

// CheckUpdates fetches the latest release (and its SHA256SUMS) for each game.
func (m *Manager) CheckUpdates(ctx context.Context) {
	var wg sync.WaitGroup
	for _, d := range m.Defs {
		wg.Add(1)
		go func(d *Def) {
			defer wg.Done()
			li := m.fetchLatest(ctx, d)
			m.mu.Lock()
			// Keep the last good answer when offline.
			if prev := m.latest[d.ID]; li.err != nil && prev != nil && prev.err == nil {
				prev.err = li.err
			} else {
				m.latest[d.ID] = li
			}
			m.mu.Unlock()
		}(d)
	}
	wg.Wait()
}

func (m *Manager) fetchLatest(ctx context.Context, d *Def) *latestInfo {
	li := &latestInfo{at: time.Now()}
	rel, err := m.GH.Latest(ctx, d.Repo)
	if err != nil {
		li.err = err
		return li
	}
	li.rel = rel
	li.version = release.NormalizeVersion(rel.Tag)
	li.asset = release.Pick(rel, d.Assets)
	if li.asset == nil {
		li.err = fmt.Errorf("release %s has no recognised game download", rel.Tag)
		return li
	}
	if li.version == "" {
		for _, p := range d.Assets {
			if sm := p.Re.FindStringSubmatch(li.asset.Name); len(sm) > 1 {
				li.version = sm[1]
				break
			}
		}
	}
	sumsAsset := rel.Asset("SHA256SUMS.txt")
	if sumsAsset == nil {
		li.err = fmt.Errorf("release %s has no SHA256SUMS.txt; the launcher won't install unverified files", rel.Tag)
		return li
	}
	sums, err := m.GH.FetchSums(ctx, sumsAsset)
	if err != nil {
		li.err = err
		return li
	}
	if _, ok := sums[li.asset.Name]; !ok {
		li.err = fmt.Errorf("SHA256SUMS.txt doesn't list %s", li.asset.Name)
		return li
	}
	li.sums = sums
	return li
}

// findExe locates the main exe in dir.
func (m *Manager) findExe(d *Def, dir string, rec state.Game) string {
	if d.Kind == KindZip {
		p := filepath.Join(dir, d.ExeName)
		if fileExists(p) {
			return p
		}
		return ""
	}
	if rec.File != "" && fileExists(filepath.Join(dir, rec.File)) {
		return filepath.Join(dir, rec.File)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type cand struct {
		path string
		rank int
		mod  time.Time
	}
	var cands []cand
	for _, e := range ents {
		if e.IsDir() || dedicated.MatchString(e.Name()) {
			continue
		}
		for rank, re := range d.ExePatterns {
			if re.MatchString(e.Name()) {
				info, _ := e.Info()
				var mt time.Time
				if info != nil {
					mt = info.ModTime()
				}
				cands = append(cands, cand{filepath.Join(dir, e.Name()), rank, mt})
				break
			}
		}
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return cands[i].mod.After(cands[j].mod)
	})
	return cands[0].path
}

// Status inspects one game. running maps game dir -> processes (from proc).
func (m *Manager) Status(id string, running map[string][]string) Status {
	d, err := m.def(id)
	if err != nil {
		return Status{ID: id, Name: id, CheckError: err.Error()}
	}
	rec := m.Store.Get(id)
	dir := m.Dir(id)
	s := Status{
		ID: d.ID, Name: d.Name, Tagline: d.Tagline, NeedsName: d.NeedsName,
		Dir: dir, Linked: rec.LinkedDir != "", SelfUpdateNote: d.SelfUpdateNote,
		Running:    len(running[dir]) > 0,
		ArtTall:    art.URL(m.L.Art, d.ID, art.Tall),
		ArtWide:    art.URL(m.L.Art, d.ID, art.Wide),
		ArtTallSrc: art.Source(m.L.Art, d.ID, art.Tall),
		ArtWideSrc: art.Source(m.L.Art, d.ID, art.Wide),
	}
	if d.UserData != nil {
		s.UserDataDir = d.UserData()
		s.UserDataExists = dirExists(s.UserDataDir)
	}

	mcc := m.MCC()
	if c := mcc.Component(d.Needs); c != nil && c.Installed {
		s.RequirementOK = true
	} else if !mcc.Found {
		s.RequirementMsg = "Halo: The Master Chief Collection (Steam) wasn't found."
	} else {
		s.RequirementMsg = d.NeedsName + " isn't installed in MCC. Install it in Steam: MCC > Manage > DLC."
	}
	if d.MustNotBeInsideMCC && mcc.Found && steam.IsInside(dir, mcc.Path) {
		s.Blocker = d.Name + " can't run from inside the MCC folder. Move the launcher to its own folder."
	}

	m.mu.Lock()
	li := m.latest[id]
	s.Busy = m.busy[id]
	m.mu.Unlock()
	var latestSHA string
	if li != nil {
		s.CheckedAt = li.at
		if li.err != nil {
			s.CheckError = li.err.Error()
		}
		if li.rel != nil {
			s.Latest, s.LatestURL, s.LatestNotes, s.LatestPublished = li.version, li.rel.HTMLURL, li.rel.Body, li.rel.PublishedAt
		}
		if li.asset != nil {
			s.LatestAsset, s.LatestSize = li.asset.Name, li.asset.Size
			latestSHA = li.sums[li.asset.Name]
		}
	}

	s.Exe = m.findExe(d, dir, rec)
	if s.Exe == "" {
		return s
	}
	s.Installed = true
	sha := m.hashExe(s.Exe)
	switch {
	case sha != "" && sha == rec.SHA256 && rec.Version != "":
		s.Version, s.VersionSource = rec.Version, "launcher"
	case d.Kind == KindExe && sha != "" && sha == latestSHA:
		// The game's own updater (or the user) installed the latest exe.
		s.Version, s.VersionSource = li.version, "release"
		if !s.Linked {
			m.Store.Update(func(st *state.State) {
				g := st.Games[id]
				if g == nil {
					g = &state.Game{}
					st.Games[id] = g
				}
				g.Version, g.SHA256, g.File, g.Asset = li.version, sha, filepath.Base(s.Exe), li.asset.Name
			})
		}
	default:
		s.UpdatedInGame = rec.SHA256 != "" && sha != rec.SHA256
		if fv := proc.FileVersion(s.Exe); fv != "" {
			s.Version, s.VersionSource = fv, "file"
		}
	}

	if s.Latest != "" && latestSHA != "" {
		switch s.VersionSource {
		case "launcher", "release":
			s.UpdateAvailable = release.Compare(s.Version, s.Latest) < 0
		case "file", "":
			// Unknown build. If the game updated itself we trust its updater
			// and don't nag; a fresh install from the launcher is still offered.
			s.UpdateAvailable = !s.UpdatedInGame && !s.Linked
		}
	}
	return s
}

// Statuses returns every game's status.
func (m *Manager) Statuses() []Status {
	dirs := make([]string, 0, len(m.Defs))
	for _, d := range m.Defs {
		dirs = append(dirs, m.Dir(d.ID))
	}
	running, _ := proc.RunningIn(dirs...)
	out := make([]Status, 0, len(m.Defs))
	for _, d := range m.Defs {
		out = append(out, m.Status(d.ID, running))
	}
	return out
}

// IsRunning reports whether anything runs from the game's folder.
func (m *Manager) IsRunning(id string) bool {
	dir := m.Dir(id)
	r, err := proc.RunningIn(dir)
	return err == nil && len(r[dir]) > 0
}

func (m *Manager) setBusy(id, msg string) {
	m.mu.Lock()
	if msg == "" {
		delete(m.busy, id)
	} else {
		m.busy[id] = msg
	}
	m.mu.Unlock()
}

// ErrRunning is returned when the game (or its updater) is running.
var ErrRunning = errors.New("the game is running; close it first")

// Install downloads, verifies and installs (or updates) a game. It writes
// only under the launcher's games folder.
func (m *Manager) Install(ctx context.Context, id string, progress func(phase string, done, total int64)) (err error) {
	d, err := m.def(id)
	if err != nil {
		return err
	}
	lock := m.locks[id]
	if !lock.TryLock() {
		return errors.New("already installing")
	}
	defer lock.Unlock()
	defer m.setBusy(id, "")
	if m.Store.Get(id).LinkedDir != "" {
		return errors.New("this game is linked to an existing folder; the launcher doesn't write there. Use the game's own updater, or unlink it to install a managed copy")
	}
	if m.IsRunning(id) {
		return ErrRunning
	}
	live := m.L.GameDir(id)
	if d.MustNotBeInsideMCC {
		if mcc := m.MCC(); mcc.Found && steam.IsInside(live, mcc.Path) {
			return errors.New(d.Name + " can't be installed inside the MCC folder; move the launcher elsewhere")
		}
	}

	m.setBusy(id, "Checking release")
	progress("check", 0, -1)
	li := m.fetchLatest(ctx, d)
	if li.err != nil {
		return li.err
	}
	want := li.sums[li.asset.Name]
	if dg := strings.TrimPrefix(li.asset.Digest, "sha256:"); dg != "" && !strings.EqualFold(dg, want) {
		return errors.New("GitHub's file digest and SHA256SUMS.txt disagree; not installing")
	}

	if err := os.MkdirAll(m.L.Staging, 0o755); err != nil {
		return err
	}
	dl := filepath.Join(m.L.Staging, id+"-"+li.asset.Name)
	defer os.Remove(dl)
	m.setBusy(id, "Downloading "+li.version)
	err = install.Download(ctx, m.HTTP, m.UA, li.asset.URL, dl, want, maxDownload, func(done, total int64) {
		progress("download", done, total)
	})
	if err != nil {
		return err
	}
	progress("verify", 1, 1)

	// Re-check right before touching the live folder: the game may have been
	// started during the download.
	if m.IsRunning(id) {
		return ErrRunning
	}
	m.setBusy(id, "Installing "+li.version)
	progress("install", 0, -1)
	prev := filepath.Join(m.L.Previous, id)
	var exe string
	switch d.Kind {
	case KindExe:
		if err := os.MkdirAll(live, 0o755); err != nil {
			return err
		}
		// Move any older client exes aside so only one is found.
		os.RemoveAll(prev)
		if ents, err := os.ReadDir(live); err == nil {
			for _, e := range ents {
				if e.IsDir() || e.Name() == li.asset.Name || dedicated.MatchString(e.Name()) {
					continue
				}
				for _, re := range d.ExePatterns {
					if re.MatchString(e.Name()) {
						os.MkdirAll(prev, 0o755)
						if err := os.Rename(filepath.Join(live, e.Name()), filepath.Join(prev, e.Name())); err != nil {
							return fmt.Errorf("can't move old %s aside (is it running?): %w", e.Name(), err)
						}
						break
					}
				}
			}
		}
		exe = filepath.Join(live, li.asset.Name)
		if err := install.ReplaceFile(dl, exe, filepath.Join(prev, li.asset.Name)); err != nil {
			return err
		}
	case KindZip:
		staged := filepath.Join(m.L.Staging, id+"-"+li.version)
		os.RemoveAll(staged)
		defer os.RemoveAll(staged)
		if err := install.Unzip(dl, staged); err != nil {
			return err
		}
		if !fileExists(filepath.Join(staged, d.ExeName)) {
			return fmt.Errorf("download doesn't contain %q; not installing", d.ExeName)
		}
		if err := os.MkdirAll(m.L.Previous, 0o755); err != nil {
			return err
		}
		if err := install.SwapDir(staged, live, prev); err != nil {
			return err
		}
		exe = filepath.Join(live, d.ExeName)
	}

	sha, err := install.HashFile(exe)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.hashes = map[string]hashEntry{}
	m.mu.Unlock()
	progress("done", 1, 1)
	return m.Store.Update(func(st *state.State) {
		st.Games[id] = &state.Game{
			Version: li.version, Asset: li.asset.Name, File: filepath.Base(exe),
			SHA256: sha, InstalledAt: time.Now().UTC(),
		}
	})
}

// Launch starts the game.
func (m *Manager) Launch(id string) error {
	d, err := m.def(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	busy := m.busy[id]
	m.mu.Unlock()
	if busy != "" {
		return errors.New("wait for the install to finish")
	}
	if m.IsRunning(id) {
		return errors.New(d.Name + " is already running")
	}
	exe := m.findExe(d, m.Dir(id), m.Store.Get(id))
	if exe == "" {
		return errors.New(d.Name + " isn't installed")
	}
	if m.BeforeLaunch != nil {
		m.BeforeLaunch(id)
	}
	return proc.Launch(exe)
}

// Join starts the game with a join link. Unlike Launch it's fine while the
// game runs: the running game takes the link over itself.
func (m *Manager) Join(id, link string) error {
	d, err := m.def(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	busy := m.busy[id]
	m.mu.Unlock()
	if busy != "" {
		return errors.New("wait for the install to finish")
	}
	exe := m.findExe(d, m.Dir(id), m.Store.Get(id))
	if exe == "" {
		return errors.New(d.Name + " isn't installed")
	}
	return proc.Launch(exe, link)
}

// Link points the launcher at an install the user already has. Nothing is
// written to that folder.
func (m *Manager) Link(id, dir string) error {
	d, err := m.def(id)
	if err != nil {
		return err
	}
	if m.findExe(d, dir, state.Game{}) == "" {
		if d.Kind == KindZip {
			return fmt.Errorf("%q not found in that folder", d.ExeName)
		}
		return errors.New("no Project Reclaimer client exe found in that folder")
	}
	if d.MustNotBeInsideMCC {
		if mcc := m.MCC(); mcc.Found && steam.IsInside(dir, mcc.Path) {
			return errors.New(d.Name + " must not be inside the MCC folder")
		}
	}
	return m.Store.Update(func(st *state.State) {
		st.Games[id] = &state.Game{LinkedDir: dir}
	})
}

// Unlink forgets a linked install (the folder itself is left alone).
func (m *Manager) Unlink(id string) error {
	return m.Store.Update(func(st *state.State) {
		if g := st.Games[id]; g != nil && g.LinkedDir != "" {
			delete(st.Games, id)
		}
	})
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
