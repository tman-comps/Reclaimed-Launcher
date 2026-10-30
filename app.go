package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reclaimed/internal/discovery"
	"reclaimed/internal/games"
	"reclaimed/internal/paths"
	"reclaimed/internal/proc"
	"reclaimed/internal/release"
	"reclaimed/internal/report"
	"reclaimed/internal/servers"
	"reclaimed/internal/settings"
	"reclaimed/internal/state"
	"reclaimed/internal/steam"
)

// Version of the launcher itself.
const Version = "0.3.0"

const userAgent = "ReclaimedLauncher/" + Version

// LauncherInfo describes the launcher's own folder.
type LauncherInfo struct {
	Version string `json:"version"`
	Root    string `json:"root"`
	Problem string `json:"problem,omitempty"`
}

// Snapshot is everything the UI draws.
type Snapshot struct {
	Launcher  LauncherInfo     `json:"launcher"`
	MCC       steam.MCCInfo    `json:"mcc"`
	Games     []games.Status   `json:"games"`
	Discovery discovery.Result `json:"discovery"`
	Checking  bool             `json:"checking"`
}

// ServerList is the server browser's state.
type ServerList struct {
	Servers   []servers.Server `json:"servers"`
	Asked     int              `json:"asked"`    // addresses the masters listed
	Answered  int              `json:"answered"` // servers that replied so far
	Done      bool             `json:"done"`
	Error     string           `json:"error,omitempty"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

// App is bound to the frontend; exported methods are callable from JS.
type App struct {
	ctx   context.Context
	L     paths.Layout
	store *state.Store
	mgr   *games.Manager
	disc  *http.Client

	mu       sync.Mutex
	mcc      steam.MCCInfo
	dr       discovery.Result
	problem  string
	checking bool
	cancels  map[string]context.CancelFunc
	sl       ServerList
	browsing bool
}

// NewApp prepares the app; startup errors are shown in the UI, not fatal.
func NewApp(l paths.Layout) *App {
	a := &App{L: l, cancels: map[string]context.CancelFunc{}, disc: &http.Client{Timeout: 6 * time.Second}}
	if err := l.CheckWritable(); err != nil {
		a.problem = err.Error()
	}
	st, err := state.Open(filepath.Join(l.Data, "state.json"))
	if err != nil {
		a.problem = "can't read launcher state: " + err.Error()
		st, _ = state.Open(filepath.Join(os.TempDir(), "reclaimed-state-unavailable.json"))
	}
	a.store = st
	a.mgr = games.NewManager(l, st, release.NewClient(userAgent), userAgent, a.MCC)
	a.mcc = steam.Detect(st.MCCPath())
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go func() {
		a.checkUpdates()
		a.refreshDiscovery()
	}()
	go a.watchRunning()
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.refreshDiscovery()
			}
		}
	}()
}

// beforeClose asks before quitting mid-install.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	n := len(a.cancels)
	a.mu.Unlock()
	if n == 0 {
		return false
	}
	res, _ := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type: runtime.QuestionDialog, Title: "Install in progress",
		Message: "A game is still installing. Quit and cancel it? Nothing half-finished is left in the game folder.",
		Buttons: []string{"Quit", "Keep going"}, DefaultButton: "Keep going", CancelButton: "Keep going",
	})
	if res != "Quit" && res != "Yes" {
		return true
	}
	a.mu.Lock()
	for _, c := range a.cancels {
		c()
	}
	a.mu.Unlock()
	return false
}

// MCC returns the last detection result.
func (a *App) MCC() steam.MCCInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mcc
}

// Snapshot returns the current state for the UI.
func (a *App) Snapshot() Snapshot {
	a.mu.Lock()
	s := Snapshot{
		Launcher:  LauncherInfo{Version: Version, Root: a.L.Root, Problem: a.problem},
		MCC:       a.mcc,
		Discovery: a.dr,
		Checking:  a.checking,
	}
	a.mu.Unlock()
	s.Games = a.mgr.Statuses()
	return s
}

func (a *App) emit() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "snapshot", a.Snapshot())
	}
}

func (a *App) checkUpdates() {
	a.mu.Lock()
	if a.checking {
		a.mu.Unlock()
		return
	}
	a.checking = true
	a.mu.Unlock()
	a.emit()
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	a.mgr.CheckUpdates(ctx)
	cancel()
	a.mu.Lock()
	a.checking = false
	a.mu.Unlock()
	a.emit()
}

func (a *App) refreshDiscovery() {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	r := discovery.Fetch(ctx, a.disc, discovery.DefaultEndpoints)
	cancel()
	a.mu.Lock()
	a.dr = r
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "discovery", r)
}

// watchRunning pushes a snapshot whenever a game starts or stops.
func (a *App) watchRunning() {
	last := map[string]bool{}
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
		}
		dirs := map[string]string{}
		var list []string
		for _, d := range games.All {
			dir := a.mgr.Dir(d.ID)
			dirs[d.ID] = dir
			list = append(list, dir)
		}
		running, err := proc.RunningIn(list...)
		if err != nil {
			continue
		}
		changed := false
		for id, dir := range dirs {
			now := len(running[dir]) > 0
			if now != last[id] {
				changed = true
				last[id] = now
			}
		}
		if changed {
			a.emit()
		}
	}
}

// ---- Bound methods -------------------------------------------------------

// CheckForUpdates re-queries GitHub for both games.
func (a *App) CheckForUpdates() { go a.checkUpdates() }

// RefreshServers re-queries the Reclaimer discovery endpoint.
func (a *App) RefreshServers() discovery.Result {
	a.refreshDiscovery()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dr
}

// RedetectMCC reruns Steam/MCC detection.
func (a *App) RedetectMCC() Snapshot {
	a.mu.Lock()
	a.mcc = steam.Detect(a.store.MCCPath())
	a.mu.Unlock()
	return a.Snapshot()
}

// ChooseMCCFolder lets the user point at MCC manually.
func (a *App) ChooseMCCFolder() (Snapshot, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select your Halo: The Master Chief Collection folder"})
	if err != nil || dir == "" {
		return a.Snapshot(), err
	}
	if c := steam.CheckComponents(dir); !c[0].Installed && !c[1].Installed {
		return a.Snapshot(), errors.New("that folder has neither halo3 nor haloreach in it")
	}
	if err := a.store.Update(func(s *state.State) { s.MCCPath = dir }); err != nil {
		return a.Snapshot(), err
	}
	return a.RedetectMCC(), nil
}

// ResetMCCFolder goes back to automatic Steam detection.
func (a *App) ResetMCCFolder() Snapshot {
	a.store.Update(func(s *state.State) { s.MCCPath = "" })
	return a.RedetectMCC()
}

// Install installs or updates a game in the background. Progress arrives as
// "progress" events; completion as "install-done".
func (a *App) Install(id string) error {
	if a.problem != "" {
		return errors.New(a.problem)
	}
	a.mu.Lock()
	if _, busy := a.cancels[id]; busy {
		a.mu.Unlock()
		return errors.New("already installing")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancels[id] = cancel
	a.mu.Unlock()
	go func() {
		err := a.mgr.Install(ctx, id, func(phase string, done, total int64) {
			runtime.EventsEmit(a.ctx, "progress", map[string]any{"id": id, "phase": phase, "done": done, "total": total})
		})
		cancel()
		a.mu.Lock()
		delete(a.cancels, id)
		a.mu.Unlock()
		msg := ""
		if err != nil {
			msg = err.Error()
			if errors.Is(err, context.Canceled) {
				msg = "cancelled"
			}
			runtime.LogErrorf(a.ctx, "install %s: %s", id, msg)
		} else {
			runtime.LogInfof(a.ctx, "install %s: done", id)
		}
		runtime.EventsEmit(a.ctx, "install-done", map[string]any{"id": id, "error": msg})
		a.emit()
	}()
	a.emit()
	return nil
}

// CancelInstall stops a running install.
func (a *App) CancelInstall(id string) {
	a.mu.Lock()
	if c := a.cancels[id]; c != nil {
		c()
	}
	a.mu.Unlock()
}

// Play launches a game.
func (a *App) Play(id string) error {
	if err := a.mgr.Launch(id); err != nil {
		return err
	}
	go func() { time.Sleep(1500 * time.Millisecond); a.emit() }()
	return nil
}

// Server browser: how many servers are asked at once, and how long each may
// take to answer. The game's own browser asks in batches too.
const (
	browseParallel = 48
	browseTimeout  = 3 * time.Second
)

// ServerList returns the server browser's last result.
func (a *App) ServerList() ServerList {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sl
}

// RefreshServerList asks every server the discovery masters list for its
// status, in the background. "servers" events carry the list as it fills.
func (a *App) RefreshServerList() {
	a.mu.Lock()
	if a.browsing {
		a.mu.Unlock()
		return
	}
	a.browsing = true
	a.mu.Unlock()
	go a.browse()
}

func (a *App) publishServers(sl ServerList) {
	servers.Sort(sl.Servers)
	sl.UpdatedAt = time.Now()
	a.mu.Lock()
	a.sl = sl
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "servers", sl)
}

func (a *App) browse() {
	defer func() {
		a.mu.Lock()
		a.browsing = false
		a.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	r := discovery.Fetch(ctx, a.disc, discovery.DefaultEndpoints)
	a.mu.Lock()
	a.dr = r
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "discovery", r)
	addrs := r.Addresses()
	sl := ServerList{Asked: len(addrs)}
	if !r.OK || len(addrs) == 0 {
		sl.Done, sl.Error = true, r.Error
		if sl.Error == "" {
			sl.Error = servers.ErrNoAddresses.Error()
		}
		a.publishServers(sl)
		return
	}
	q, err := servers.NewQuerier()
	if err != nil {
		sl.Done, sl.Error = true, err.Error()
		a.publishServers(sl)
		return
	}
	defer q.Close()
	a.publishServers(sl)
	last := time.Now()
	q.Browse(ctx, addrs, browseParallel, browseTimeout, func(s servers.Server, err error) {
		if err != nil {
			return
		}
		sl.Answered++
		sl.Servers = append(sl.Servers, s)
		if time.Since(last) > 400*time.Millisecond {
			last = time.Now()
			a.publishServers(ServerList{Servers: append([]servers.Server(nil), sl.Servers...), Asked: sl.Asked, Answered: sl.Answered})
		}
	})
	sl.Done = true
	runtime.LogInfof(a.ctx, "servers: %d of %d answered", sl.Answered, sl.Asked)
	a.publishServers(sl)
}

// JoinServer starts Project Reclaimer with a join link to addr (host:port).
// The game does the joining; a running game takes the link over itself.
func (a *App) JoinServer(addr string) error {
	addr = strings.TrimSpace(addr)
	if !servers.ValidAddress(addr) {
		return errors.New("enter the server's address as host:port, for example 1.2.3.4:49177")
	}
	if err := a.mgr.Join("reclaimer", servers.JoinLink(addr)); err != nil {
		return err
	}
	runtime.LogInfof(a.ctx, "join: asked Project Reclaimer to join %s", addr)
	go func() { time.Sleep(1500 * time.Millisecond); a.emit() }()
	return nil
}

// LinkExisting uses an install the user already has (read/launch only).
func (a *App) LinkExisting(id string) (Snapshot, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select the folder that contains the game"})
	if err != nil || dir == "" {
		return a.Snapshot(), err
	}
	if err := a.mgr.Link(id, dir); err != nil {
		return a.Snapshot(), err
	}
	return a.Snapshot(), nil
}

// Unlink forgets a linked install without touching it.
func (a *App) Unlink(id string) (Snapshot, error) {
	return a.Snapshot(), a.mgr.Unlink(id)
}

// OpenGameFolder shows the game folder in Explorer.
func (a *App) OpenGameFolder(id string) error { return proc.OpenFolder(a.mgr.Dir(id)) }

// OpenDataFolder shows the game's own data/log folder in Explorer.
func (a *App) OpenDataFolder(id string) error {
	d := games.ByID(id)
	if d == nil || d.UserData == nil {
		return errors.New("unknown game")
	}
	p := d.UserData()
	if _, err := os.Stat(p); err != nil {
		return errors.New("the game hasn't created " + p + " yet; run it once")
	}
	return proc.OpenFolder(p)
}

// OpenArtFolder shows the cover-art folder, creating it if needed (it lives
// inside the launcher's folder).
func (a *App) OpenArtFolder() error {
	if err := os.MkdirAll(a.L.Art, 0o755); err != nil {
		return err
	}
	return proc.OpenFolder(a.L.Art)
}

// OpenURL opens a link in the default browser.
func (a *App) OpenURL(url string) { runtime.BrowserOpenURL(a.ctx, url) }

// SettingsSaved is the result of a settings save.
type SettingsSaved struct {
	Page   settings.Page `json:"page"`
	Backup string        `json:"backup"` // "" when nothing needed writing
}

func (a *App) settingsGame(id string) (*games.Def, error) {
	d := games.ByID(id)
	if d == nil || !settings.Supported(id) {
		return nil, errors.New("unknown game")
	}
	return d, nil
}

func (a *App) settingsBackups(id string) string { return filepath.Join(a.L.Data, "backups", id) }

// GameSettings reads a game's own settings file for the Settings page.
func (a *App) GameSettings(id string) (settings.Page, error) {
	d, err := a.settingsGame(id)
	if err != nil {
		return settings.Page{}, err
	}
	return settings.Load(id, d.UserData()), nil
}

// SaveGameSettings writes changed settings (key -> value) to the game's file,
// if it still matches the page loaded with stamp. It refuses while the game
// runs and backs the file up into data\backups first.
func (a *App) SaveGameSettings(id, stamp string, changes map[string]string) (SettingsSaved, error) {
	d, err := a.settingsGame(id)
	if err != nil {
		return SettingsSaved{}, err
	}
	if a.mgr.IsRunning(id) {
		return SettingsSaved{}, errors.New(d.Name + " is running. Close it first; its settings weren't changed")
	}
	p, backup, err := settings.Save(id, d.UserData(), a.settingsBackups(id), stamp, changes, time.Now())
	if err != nil {
		runtime.LogErrorf(a.ctx, "settings %s: not saved: %v", id, err)
		return SettingsSaved{Page: p}, err
	}
	if backup != "" {
		runtime.LogInfof(a.ctx, "settings %s: saved %d change(s) to %s; backup %s", id, len(changes), p.Path, backup)
	}
	return SettingsSaved{Page: p, Backup: backup}, nil
}

// SettingsCompare lines up the settings both games share.
type SettingsCompare struct {
	Rows  []settings.SyncRow `json:"rows"`
	Error string             `json:"error,omitempty"`
}

func (a *App) bothPages() (settings.Page, settings.Page) {
	r, _ := a.GameSettings("reclaimer")
	b, _ := a.GameSettings("reach-renaissance")
	return r, b
}

// CompareSettings reads both games' settings and compares the shared ones.
func (a *App) CompareSettings() SettingsCompare {
	r, b := a.bothPages()
	for _, p := range []settings.Page{r, b} {
		if p.Error != "" {
			return SettingsCompare{Error: games.ByID(p.Game).Name + ": " + p.Error}
		}
	}
	return SettingsCompare{Rows: settings.Compare(r, b)}
}

// CopySettings copies shared settings (all that differ when keys is empty)
// from one game to the other, as one ordinary save of the other game.
func (a *App) CopySettings(from, to string, keys []string) (SettingsSaved, error) {
	if from == to {
		return SettingsSaved{}, errors.New("pick two different games")
	}
	pages := map[string]settings.Page{}
	pages["reclaimer"], pages["reach-renaissance"] = a.bothPages()
	src, ok1 := pages[from]
	dst, ok2 := pages[to]
	if !ok1 || !ok2 {
		return SettingsSaved{}, errors.New("unknown game")
	}
	changes, err := settings.CopyChanges(src, dst, keys)
	if err != nil {
		return SettingsSaved{}, err
	}
	if len(changes) == 0 {
		return SettingsSaved{Page: dst}, nil
	}
	runtime.LogInfof(a.ctx, "settings: copying %d setting(s) from %s to %s", len(changes), from, to)
	return a.SaveGameSettings(to, dst.Stamp, changes)
}

// OpenSettingsBackups opens the folder holding a game's settings backups.
func (a *App) OpenSettingsBackups(id string) error {
	if _, err := a.settingsGame(id); err != nil {
		return err
	}
	dir := a.settingsBackups(id)
	if _, err := os.Stat(dir); err != nil {
		return errors.New("no backups yet: one is made before each save")
	}
	return proc.OpenFolder(dir)
}

// ExportSettingsReport writes a read-only inventory of both games' settings
// folders into the launcher's reports folder (Phase 2 investigation).
func (a *App) ExportSettingsReport() (string, error) {
	var targets []report.Target
	for _, d := range games.All {
		targets = append(targets, report.Target{Game: d.Name, Dir: d.UserData()})
		if dir := a.mgr.Dir(d.ID); dir != "" {
			targets = append(targets, report.Target{Game: d.Name + " (game folder)", Dir: dir})
		}
	}
	p, err := report.Build(a.L.Reports, targets)
	if err == nil {
		proc.OpenFolder(a.L.Reports)
	}
	return p, err
}
