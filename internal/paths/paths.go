// Package paths defines the launcher's folder layout. Everything the
// launcher writes lives under Root; game user-data folders are only read.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Layout is the on-disk layout under the launcher's folder.
type Layout struct {
	Root     string // folder containing the launcher exe
	Games    string // <root>/games/<game id>
	Staging  string // <root>/games/.staging (downloads in progress)
	Previous string // <root>/games/.previous/<game id> (last version, for rollback)
	Data     string // <root>/data (state.json, logs, webview cache)
	Reports  string // <root>/reports
	Art      string // <root>/art (optional user-supplied cover images)
}

// Resolve builds the layout from the exe location. RALLY_HOME overrides it
// for development.
func Resolve() (Layout, error) {
	root := os.Getenv("RALLY_HOME")
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			return Layout{}, err
		}
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
		root = filepath.Dir(exe)
	}
	return New(root), nil
}

// New returns the layout for root.
func New(root string) Layout {
	games := filepath.Join(root, "games")
	return Layout{
		Root:     root,
		Games:    games,
		Staging:  filepath.Join(games, ".staging"),
		Previous: filepath.Join(games, ".previous"),
		Data:     filepath.Join(root, "data"),
		Reports:  filepath.Join(root, "reports"),
		Art:      filepath.Join(root, "art"),
	}
}

// GameDir is where a game is installed by the launcher.
func (l Layout) GameDir(id string) string { return filepath.Join(l.Games, id) }

// CheckWritable makes sure the launcher can write next to itself, and warns
// about locations where that is a bad idea.
func (l Layout) CheckWritable() error {
	low := strings.ToLower(l.Root)
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "SystemRoot"} {
		if p := os.Getenv(env); p != "" && strings.HasPrefix(low, strings.ToLower(p)+string(filepath.Separator)) {
			return errors.New("the launcher is inside " + p + ". Move it to its own folder, e.g. C:\\Games\\Rally, so it can install games next to itself without admin rights")
		}
	}
	if err := os.MkdirAll(l.Data, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(l.Data, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return errors.New("can't write to the launcher's folder: " + err.Error())
	}
	return os.Remove(probe)
}
