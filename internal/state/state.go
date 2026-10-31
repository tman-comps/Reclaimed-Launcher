// Package state persists what the launcher installed, in data/state.json.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Game records one install made (or linked) by the launcher.
type Game struct {
	Version     string    `json:"version,omitempty"`
	Asset       string    `json:"asset,omitempty"`  // release asset the install came from
	File        string    `json:"file,omitempty"`   // main exe, relative to the game folder
	SHA256      string    `json:"sha256,omitempty"` // of File at install time
	InstalledAt time.Time `json:"installedAt,omitempty"`
	// LinkedDir is an install the user already had elsewhere. The launcher
	// only reads and launches it; it never writes there.
	LinkedDir string `json:"linkedDir,omitempty"`
}

// State is the whole file.
type State struct {
	Version int              `json:"version"`
	MCCPath string           `json:"mccPath,omitempty"` // manual override
	Games   map[string]*Game `json:"games"`
	// FirstRunDone lists games whose first-run settings were applied
	// (settings.ApplyFirstRun), so they're never applied twice.
	FirstRunDone map[string]bool `json:"firstRunDone,omitempty"`
	// Friends are Project Reclaimer player names the server browser
	// looks for.
	Friends []string `json:"friends,omitempty"`
	// Favorites and Recent are Project Reclaimer servers the player starred
	// or joined.
	Favorites []SavedServer `json:"favorites,omitempty"`
	Recent    []SavedServer `json:"recent,omitempty"`
}

// Store guards State and saves it atomically.
type Store struct {
	path string
	mu   sync.Mutex
	s    State
}

// Open loads path, starting empty if it doesn't exist. A corrupt file is
// renamed aside rather than overwritten.
func Open(path string) (*Store, error) {
	st := &Store{path: path, s: State{Version: 1, Games: map[string]*Game{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &st.s); err != nil {
		os.Rename(path, path+".corrupt-"+time.Now().Format("20060102-150405"))
		st.s = State{Version: 1, Games: map[string]*Game{}}
		return st, nil
	}
	if st.s.Games == nil {
		st.s.Games = map[string]*Game{}
	}
	return st, nil
}

// Get returns a copy of a game's record (zero value if none).
func (st *Store) Get(id string) Game {
	st.mu.Lock()
	defer st.mu.Unlock()
	if g := st.s.Games[id]; g != nil {
		return *g
	}
	return Game{}
}

// FirstRunDone reports whether a game's first-run settings were applied.
func (st *Store) FirstRunDone(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s.FirstRunDone[id]
}

// SetFirstRunDone records that a game's first-run settings were applied.
func (st *Store) SetFirstRunDone(id string) error {
	return st.Update(func(s *State) {
		if s.FirstRunDone == nil {
			s.FirstRunDone = map[string]bool{}
		}
		s.FirstRunDone[id] = true
	})
}

// Friends returns the saved friend names.
func (st *Store) Friends() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]string{}, st.s.Friends...)
}

// SetFriends saves the friend names: trimmed, without blanks or
// case-insensitive duplicates, in the given order, at most 200.
func (st *Store) SetFriends(names []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || len(n) > 64 || seen[strings.ToLower(n)] || len(out) >= 200 {
			continue
		}
		seen[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out, st.Update(func(s *State) { s.Friends = out })
}

// MCCPath returns the manual MCC override.
func (st *Store) MCCPath() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s.MCCPath
}

// Update mutates state under lock and saves.
func (st *Store) Update(fn func(s *State)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	fn(&st.s)
	return st.save()
}

func (st *Store) save() error {
	b, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// SavedServer is a Project Reclaimer server the player starred or joined.
type SavedServer struct {
	Address string    `json:"address"`
	Name    string    `json:"name,omitempty"`
	At      time.Time `json:"at,omitempty"` // when it was last joined (recent list)
}

// MaxRecent is how many recently joined servers are kept.
const MaxRecent = 8

// ServerPrefs returns the starred servers and the recently joined ones
// (newest first).
func (st *Store) ServerPrefs() (favorites, recent []SavedServer) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]SavedServer{}, st.s.Favorites...), append([]SavedServer{}, st.s.Recent...)
}

// ToggleFavorite stars a server, or unstars it if it was starred.
func (st *Store) ToggleFavorite(addr, name string) error {
	return st.Update(func(s *State) {
		for i, f := range s.Favorites {
			if strings.EqualFold(f.Address, addr) {
				s.Favorites = append(s.Favorites[:i], s.Favorites[i+1:]...)
				return
			}
		}
		if len(s.Favorites) < 200 {
			s.Favorites = append(s.Favorites, SavedServer{Address: addr, Name: name})
		}
	})
}

// AddRecent records a join, moving the server to the front of the list.
func (st *Store) AddRecent(addr, name string, now time.Time) error {
	return st.Update(func(s *State) {
		out := []SavedServer{{Address: addr, Name: name, At: now}}
		for _, r := range s.Recent {
			if !strings.EqualFold(r.Address, addr) && len(out) < MaxRecent {
				out = append(out, r)
			}
		}
		s.Recent = out
	})
}
