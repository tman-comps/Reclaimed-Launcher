package steam

import (
	"embed"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"reclaimed/internal/vdf"
)

// Shortcut is a non-Steam game in the player's Steam library.
type Shortcut struct {
	Name     string
	Exe      string
	StartDir string
	Icon     string
}

// ErrNoSteamUser means no Steam account has signed in on this PC yet.
var ErrNoSteamUser = errors.New("no Steam account has signed in on this PC yet; open Steam and sign in once")

// steamID64Base turns a 64-bit SteamID into the account ID used for the
// userdata folder.
const steamID64Base = 76561197960265728

// CurrentUser returns the account ID (the userdata folder name) of the
// account that signed in to Steam most recently.
func CurrentUser(root string) (string, error) {
	var ids []string
	if b, err := os.ReadFile(filepath.Join(root, "config", "loginusers.vdf")); err == nil {
		if doc, err := vdf.Parse(string(b)); err == nil {
			if users := doc.Get("users"); users != nil {
				for _, k := range users.Keys() {
					id64, err := strconv.ParseUint(k, 10, 64)
					if err != nil || id64 < steamID64Base {
						continue
					}
					id := strconv.FormatUint(id64-steamID64Base, 10)
					if users.String(k, "MostRecent") == "1" {
						return id, nil
					}
					ids = append(ids, id)
				}
			}
		}
	}
	// No "most recent" mark: use the only signed-in account, if there's one.
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) == 0 {
		ents, _ := os.ReadDir(filepath.Join(root, "userdata"))
		for _, e := range ents {
			if _, err := strconv.ParseUint(e.Name(), 10, 32); e.IsDir() && err == nil && e.Name() != "0" {
				ids = append(ids, e.Name())
			}
		}
		if len(ids) == 1 {
			return ids[0], nil
		}
	}
	if len(ids) == 0 {
		return "", ErrNoSteamUser
	}
	return "", errors.New("more than one Steam account has used this PC and none is marked as the last one; sign in to Steam with yours and try again")
}

// ShortcutsFile is the user's list of non-Steam games.
func ShortcutsFile(root, user string) string {
	return filepath.Join(root, "userdata", user, "config", "shortcuts.vdf")
}

// ShortcutAppID is the ID Steam gives a non-Steam game (used for its
// library artwork), from the quoted exe path and the name.
func ShortcutAppID(quotedExe, name string) uint32 {
	return crc32.ChecksumIEEE([]byte(quotedExe+name)) | 0x80000000
}

func quote(p string) string { return `"` + p + `"` }

// AddShortcut adds s to the user's non-Steam games unless an entry already
// starts the same exe. Steam must be closed: it rewrites the file when it
// exits, which would drop the new entry. The old file is copied to
// backupDir first. added is false when the shortcut was already there.
func AddShortcut(root, user string, s Shortcut, backupDir string, now time.Time) (added bool, err error) {
	path := ShortcutsFile(root, user)
	data, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return false, err
	}
	var doc *vdf.BinNode
	if missing {
		doc = &vdf.BinNode{}
		doc.Add(vdf.BinMap("shortcuts"))
	} else if doc, err = vdf.ParseBinary(data); err != nil {
		return false, fmt.Errorf("Steam's shortcut list isn't in a format the launcher knows, so it was left alone: %w", err)
	}
	list := doc.Child("shortcuts")
	if list == nil || !list.IsMap() {
		return false, errors.New("Steam's shortcut list has no shortcuts section, so it was left alone")
	}
	next := 0
	for _, e := range list.Children {
		if n, err := strconv.Atoi(e.Key); err == nil && n >= next {
			next = n + 1
		}
		if exe := e.Child("Exe"); exe != nil && strings.EqualFold(strings.Trim(exe.Str, `"`), s.Exe) {
			return false, nil
		}
	}
	e := vdf.BinMap(strconv.Itoa(next))
	e.AddInt("appid", ShortcutAppID(quote(s.Exe), s.Name))
	e.AddString("AppName", s.Name)
	e.AddString("Exe", quote(s.Exe))
	e.AddString("StartDir", quote(s.StartDir))
	e.AddString("icon", s.Icon)
	e.AddString("ShortcutPath", "")
	e.AddString("LaunchOptions", "")
	e.AddInt("IsHidden", 0)
	e.AddInt("AllowDesktopConfig", 1)
	e.AddInt("AllowOverlay", 1)
	e.AddInt("OpenVR", 0)
	e.AddInt("Devkit", 0)
	e.AddString("DevkitGameID", "")
	e.AddInt("DevkitOverrideAppID", 0)
	e.AddInt("LastPlayTime", 0)
	e.AddString("FlatpakAppID", "")
	e.Add(vdf.BinMap("tags"))
	list.Add(e)
	out := doc.Bytes()
	if _, err := vdf.ParseBinary(out); err != nil {
		return false, fmt.Errorf("internal error: the new shortcut list wouldn't be readable (%v); nothing was written", err)
	}
	if !missing {
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			return false, err
		}
		b := filepath.Join(backupDir, "shortcuts.vdf."+now.Format("20060102-150405")+".bak")
		if err := os.WriteFile(b, data, 0o644); err != nil {
			return false, fmt.Errorf("couldn't back up Steam's shortcut list, so nothing was written: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	tmp := path + ".reclaimed-tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		os.Remove(tmp)
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	return true, nil
}

// FindShortcut returns the Steam ID of the user's non-Steam game that starts
// exe, if there is one. It only reads, so Steam may be running.
func FindShortcut(root, user, exe string) (appID uint32, found bool, err error) {
	data, err := os.ReadFile(ShortcutsFile(root, user))
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	doc, err := vdf.ParseBinary(data)
	if err != nil {
		return 0, false, err
	}
	list := doc.Child("shortcuts")
	if list == nil {
		return 0, false, nil
	}
	for _, e := range list.Children {
		x := e.Child("Exe")
		if x == nil || !strings.EqualFold(strings.Trim(x.Str, `"`), exe) {
			continue
		}
		if id := e.Child("appid"); id != nil && id.Int != 0 {
			return id.Int, true, nil
		}
		name := ""
		if n := e.Child("AppName"); n != nil {
			name = n.Str
		}
		return ShortcutAppID(x.Str, name), true, nil // older entries without an appid
	}
	return 0, false, nil
}

//go:embed art
var gridArt embed.FS

// gridFiles maps the launcher's art to Steam's grid file names: library
// cover, wide capsule, page header and the logo drawn over it.
var gridFiles = []struct{ src, suffix string }{
	{"art/portrait.jpg", "p"},
	{"art/wide.jpg", ""},
	{"art/hero.jpg", "_hero"},
	{"art/logo.png", "_logo"},
}

// WriteGridArt gives the non-Steam game appID the launcher's library art,
// in the user's Steam grid folder. A slot that already has an image (set by
// the player or a tool) is left alone. It returns how many images it wrote.
func WriteGridArt(root, user string, appID uint32) (int, error) {
	dir := filepath.Join(root, "userdata", user, "config", "grid")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	id := strconv.FormatUint(uint64(appID), 10)
	n := 0
	for _, f := range gridFiles {
		base := id + f.suffix
		if m, _ := filepath.Glob(filepath.Join(dir, base+".*")); len(m) > 0 {
			continue
		}
		data, err := gridArt.ReadFile(f.src)
		if err != nil {
			return n, err
		}
		if err := os.WriteFile(filepath.Join(dir, base+filepath.Ext(f.src)), data, 0o644); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
