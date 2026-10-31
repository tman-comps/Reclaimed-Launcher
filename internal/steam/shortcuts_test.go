package steam

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reclaimed/internal/vdf"
)

const loginUsers = `"users"
{
	"76561197960265729"
	{
		"AccountName"		"someone"
		"MostRecent"		"0"
	}
	"76561197960275728"
	{
		"AccountName"		"player"
		"MostRecent"		"1"
	}
}
`

func TestCurrentUser(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "config"), 0o755)
	os.WriteFile(filepath.Join(root, "config", "loginusers.vdf"), []byte(loginUsers), 0o644)
	if id, err := CurrentUser(root); err != nil || id != "10000" {
		t.Fatal(id, err)
	}

	// Without loginusers.vdf, a single userdata folder is used.
	root = t.TempDir()
	os.MkdirAll(filepath.Join(root, "userdata", "0"), 0o755)
	os.MkdirAll(filepath.Join(root, "userdata", "4242"), 0o755)
	if id, err := CurrentUser(root); err != nil || id != "4242" {
		t.Fatal(id, err)
	}
	if _, err := CurrentUser(t.TempDir()); err != ErrNoSteamUser {
		t.Fatal(err)
	}
}

func TestAddShortcut(t *testing.T) {
	root, backups := t.TempDir(), filepath.Join(t.TempDir(), "backups")
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s := Shortcut{Name: "Reclaimed", Exe: `C:\Games\Reclaimed\Reclaimed.exe`, StartDir: `C:\Games\Reclaimed`, Icon: `C:\Games\Reclaimed\Reclaimed.exe`}

	// No file yet: one is created, nothing to back up.
	if added, err := AddShortcut(root, "4242", s, backups, now); !added || err != nil {
		t.Fatal(added, err)
	}
	data, err := os.ReadFile(ShortcutsFile(root, "4242"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := vdf.ParseBinary(data)
	if err != nil {
		t.Fatal(err)
	}
	e := doc.Child("shortcuts").Child("0")
	if e.Child("AppName").Str != "Reclaimed" || e.Child("Exe").Str != `"C:\Games\Reclaimed\Reclaimed.exe"` ||
		e.Child("appid").Int != ShortcutAppID(`"C:\Games\Reclaimed\Reclaimed.exe"`, "Reclaimed") || e.Child("appid").Int&0x80000000 == 0 {
		t.Fatalf("entry: %+v", e.Children)
	}

	// Again: already there, file untouched.
	if added, err := AddShortcut(root, "4242", s, backups, now); added || err != nil {
		t.Fatal(added, err)
	}
	if again, _ := os.ReadFile(ShortcutsFile(root, "4242")); string(again) != string(data) {
		t.Fatal("file changed")
	}

	// A second exe is added after the existing entry, with a backup.
	s2 := s
	s2.Exe, s2.Name = `D:\Other\Reclaimed.exe`, "Reclaimed (2)"
	if added, err := AddShortcut(root, "4242", s2, backups, now); !added || err != nil {
		t.Fatal(added, err)
	}
	data, _ = os.ReadFile(ShortcutsFile(root, "4242"))
	doc, _ = vdf.ParseBinary(data)
	if doc.Child("shortcuts").Child("1") == nil || doc.Child("shortcuts").Child("0").Child("AppName").Str != "Reclaimed" {
		t.Fatal("existing entry lost or new one misplaced")
	}
	if m, _ := filepath.Glob(filepath.Join(backups, "shortcuts.vdf.*.bak")); len(m) != 1 {
		t.Fatalf("backups: %v", m)
	}

	// A file the launcher can't read is left alone.
	os.WriteFile(ShortcutsFile(root, "4242"), []byte("not vdf"), 0o644)
	if _, err := AddShortcut(root, "4242", s, backups, now); err == nil {
		t.Fatal("want an error")
	}
}

func TestFindShortcutAndGridArt(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	exe := `C:\Games\Reclaimed\Reclaimed.exe`
	if _, found, err := FindShortcut(root, "4242", exe); found || err != nil {
		t.Fatal(found, err)
	}
	AddShortcut(root, "4242", Shortcut{Name: "Reclaimed", Exe: exe, StartDir: `C:\Games\Reclaimed`}, t.TempDir(), now)
	id, found, err := FindShortcut(root, "4242", `c:\games\reclaimed\RECLAIMED.exe`)
	if !found || err != nil || id != ShortcutAppID(`"`+exe+`"`, "Reclaimed") {
		t.Fatal(id, found, err)
	}

	grid := filepath.Join(root, "userdata", "4242", "config", "grid")
	os.MkdirAll(grid, 0o755)
	custom := filepath.Join(grid, fmt.Sprintf("%dp.png", id)) // the player's own cover
	os.WriteFile(custom, []byte("mine"), 0o644)
	n, err := WriteGridArt(root, "4242", id)
	if err != nil || n != 3 {
		t.Fatal(n, err)
	}
	if b, _ := os.ReadFile(custom); string(b) != "mine" {
		t.Fatal("the player's cover was replaced")
	}
	for _, name := range []string{"%d.jpg", "%d_hero.jpg", "%d_logo.png"} {
		if st, err := os.Stat(filepath.Join(grid, fmt.Sprintf(name, id))); err != nil || st.Size() < 1000 {
			t.Fatal(name, err)
		}
	}
	if n, _ := WriteGridArt(root, "4242", id); n != 0 {
		t.Fatal("wrote again:", n)
	}
}
