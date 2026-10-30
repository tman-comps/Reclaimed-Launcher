package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"rally/internal/discovery"
	"rally/internal/games"
	"rally/internal/paths"
	"rally/internal/proc"
	"rally/internal/release"
	"rally/internal/report"
	"rally/internal/servers"
	"rally/internal/state"
	"rally/internal/steam"
)

// Version of the launcher itself.
const Version = "0.2.1"

const userAgent = "RallyLauncher/" + Version

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
	Servers   servers.Status   `json:"servers"`
	Checking  bool             `json:"checking"`
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
		st, _ = state.Open(filepath.Join(os.TempDir(), "rally-state-unavailable.json"))
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
		Servers:   servers.Available(),
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
