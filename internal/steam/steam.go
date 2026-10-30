// Package steam finds the Halo: The Master Chief Collection install across
// all Steam libraries. Everything here is read-only.
package steam

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"reclaimed/internal/vdf"
)

// MCCAppID is Steam's app id for Halo: The Master Chief Collection.
const MCCAppID = "976730"

// Component is one game inside MCC that a client depends on.
type Component struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Detail    string `json:"detail"`
}

// MCCInfo is the result of a detection pass.
type MCCInfo struct {
	SteamFound bool        `json:"steamFound"`
	SteamPath  string      `json:"steamPath"`
	Libraries  []string    `json:"libraries"`
	Found      bool        `json:"found"`
	Path       string      `json:"path"`
	Source     string      `json:"source"` // "steam" or "manual"
	Components []Component `json:"components"`
	Error      string      `json:"error,omitempty"`
}

// Component returns a component by id, or nil.
func (m *MCCInfo) Component(id string) *Component {
	for i := range m.Components {
		if m.Components[i].ID == id {
			return &m.Components[i]
		}
	}
	return nil
}

// componentChecks: an MCC game counts as installed when its folder has the
// engine dll and a maps folder with at least one .map file.
var componentChecks = []struct{ id, name, dir string }{
	{"halo3", "Halo 3", "halo3"},
	{"haloreach", "Halo: Reach", "haloreach"},
}

// Libraries reads libraryfolders.vdf under steamRoot and returns each library
// root (the steam root is always included first).
func Libraries(steamRoot string) []string {
	libs := []string{steamRoot}
	seen := map[string]bool{strings.ToLower(filepath.Clean(steamRoot)): true}
	for _, p := range []string{
		filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		filepath.Join(steamRoot, "config", "libraryfolders.vdf"),
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		root, err := vdf.Parse(string(b))
		if err != nil {
			continue
		}
		lf := root.Get("libraryfolders")
		if lf == nil {
			continue
		}
		for _, k := range lf.Keys() {
			e := lf.Get(k)
			path := e.String("path")
			if path == "" && e.Children == nil {
				path = e.Value // very old format: "1" "D:\\SteamLibrary"
			}
			if path == "" {
				continue
			}
			key := strings.ToLower(filepath.Clean(path))
			if !seen[key] {
				seen[key] = true
				libs = append(libs, path)
			}
		}
	}
	return libs
}

// FindMCC looks through libraries for MCC's app manifest and returns its
// install folder.
func FindMCC(libraries []string) (string, error) {
	for _, lib := range libraries {
		manifest := filepath.Join(lib, "steamapps", "appmanifest_"+MCCAppID+".acf")
		b, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}
		root, err := vdf.Parse(string(b))
		if err != nil {
			continue
		}
		dir := root.String("AppState", "installdir")
		if dir == "" {
			dir = "Halo The Master Chief Collection"
		}
		full := filepath.Join(lib, "steamapps", "common", dir)
		if st, err := os.Stat(full); err == nil && st.IsDir() {
			return full, nil
		}
	}
	return "", errors.New("MCC app manifest not found in any Steam library")
}

// CheckComponents inspects an MCC folder for Halo 3 and Halo: Reach.
func CheckComponents(mccPath string) []Component {
	out := make([]Component, 0, len(componentChecks))
	for _, c := range componentChecks {
		comp := Component{ID: c.id, Name: c.name}
		dir := filepath.Join(mccPath, c.dir)
		dll := filepath.Join(dir, c.dir+".dll")
		maps, _ := filepath.Glob(filepath.Join(dir, "maps", "*.map"))
		_, dllErr := os.Stat(dll)
		switch {
		case dllErr == nil && len(maps) > 0:
			comp.Installed = true
			comp.Detail = dir
		case dllErr == nil:
			comp.Detail = "engine found but no maps; install it in Steam (Manage > DLC)"
		default:
			comp.Detail = "not installed in MCC; install it in Steam (Manage > DLC)"
		}
		out = append(out, comp)
	}
	return out
}

// steamRoot is SteamRoot, swappable so tests don't read the real registry.
var steamRoot = SteamRoot

// Detect runs a full detection. manualPath, when set, overrides Steam lookup.
func Detect(manualPath string) MCCInfo {
	info := MCCInfo{}
	if root := steamRoot(); root != "" {
		info.SteamFound = true
		info.SteamPath = root
		info.Libraries = Libraries(root)
	}
	if manualPath != "" {
		info.Path, info.Source = manualPath, "manual"
	} else if info.SteamFound {
		p, err := FindMCC(info.Libraries)
		if err != nil {
			info.Error = err.Error()
		}
		info.Path, info.Source = p, "steam"
	} else {
		info.Error = "Steam not found"
	}
	if info.Path != "" {
		if st, err := os.Stat(info.Path); err == nil && st.IsDir() {
			info.Found = true
			info.Components = CheckComponents(info.Path)
		} else {
			info.Error = "MCC folder does not exist: " + info.Path
		}
	}
	if info.Components == nil {
		for _, c := range componentChecks {
			info.Components = append(info.Components, Component{ID: c.id, Name: c.name, Detail: "MCC not found"})
		}
	}
	return info
}

// IsInside reports whether path is inside (or equal to) dir.
func IsInside(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
