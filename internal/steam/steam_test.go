package steam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSteamRoot points Detect at root instead of the registry or real Steam.
func fakeSteamRoot(t *testing.T, root string) {
	t.Helper()
	old := steamRoot
	steamRoot = func() string { return root }
	t.Cleanup(func() { steamRoot = old })
}

func TestDetectAcrossLibraries(t *testing.T) {
	tmp := t.TempDir()
	steamRoot := filepath.Join(tmp, "Steam")
	lib2 := filepath.Join(tmp, "SteamLibrary")
	esc := strings.ReplaceAll(lib2, `\`, `\\`)
	write(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+strings.ReplaceAll(steamRoot, `\`, `\\`)+`" } "1" { "path" "`+esc+`" } }`)
	write(t, filepath.Join(lib2, "steamapps", "appmanifest_976730.acf"),
		`"AppState" { "appid" "976730" "installdir" "Halo The Master Chief Collection" }`)
	mcc := filepath.Join(lib2, "steamapps", "common", "Halo The Master Chief Collection")
	write(t, filepath.Join(mcc, "halo3", "halo3.dll"), "x")
	write(t, filepath.Join(mcc, "halo3", "maps", "guardian.map"), "x")
	write(t, filepath.Join(mcc, "haloreach", "haloreach.dll"), "x") // no maps

	fakeSteamRoot(t, steamRoot)
	info := Detect("")
	if !info.Found || info.Path != mcc {
		t.Fatalf("MCC not found: %+v", info)
	}
	if len(info.Libraries) != 2 {
		t.Fatalf("libraries = %v", info.Libraries)
	}
	if !info.Component("halo3").Installed {
		t.Error("halo3 should be installed")
	}
	if info.Component("haloreach").Installed {
		t.Error("reach without maps should not count as installed")
	}
}

func TestDetectNoSteam(t *testing.T) {
	fakeSteamRoot(t, "")
	info := Detect("")
	if info.Found || info.SteamFound || len(info.Components) != 2 {
		t.Fatalf("unexpected: %+v", info)
	}
}

func TestIsInside(t *testing.T) {
	base := filepath.Join("a", "mcc")
	cases := map[string]bool{
		base:                            true,
		filepath.Join(base, "x", "y"):   true,
		filepath.Join("a", "mccother"):  false,
		filepath.Join("a", "elsewhere"): false,
	}
	for p, want := range cases {
		if got := IsInside(p, base); got != want {
			t.Errorf("IsInside(%q) = %v", p, got)
		}
	}
}
