package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The file Project Reclaimer v0.9.23 wrote on the first test PC, plus an
// unknown section to prove it survives.
const reclaimerFile = `{
  "version": 1,
  "mouse_sensitivity": 1.5,
  "field_of_view": 85.0,
  "vehicle_field_of_view": 70.0,
  "mouse_smoothing_ms": 6.0,
  "graphics": {
    "display_mode": "windowed",
    "width": 3840,
    "height": 2160,
    "frame_limit": 120,
    "render_resolution": "r2160p",
    "vsync": true
  },
  "audio": {
    "master_volume": 1.0,
    "muted": false,
    "mute_unfocused": false,
    "main_menu_music": true
  },
  "future_section": {
    "z": 1,
    "a": [1, 2]
  }
}`

var now = time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)

func setup(t *testing.T, name, content string) (dir, backups string) {
	t.Helper()
	root := t.TempDir()
	dir, backups = filepath.Join(root, "game"), filepath.Join(root, "backups")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, backups
}

func field(t *testing.T, p Page, key string) Field {
	t.Helper()
	for _, f := range p.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("no field %q", key)
	return Field{}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReclaimerLoad(t *testing.T) {
	dir, _ := setup(t, "settings.json", reclaimerFile)
	p := Load("reclaimer", dir)
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	for key, want := range map[string]string{
		"graphics.display_mode": "windowed", "graphics.vsync": "on", "audio.master_volume": "100",
		"field_of_view": "85", "graphics.frame_limit": "120", "audio.muted": "off",
	} {
		if got := field(t, p, key).Value; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if !field(t, p, "field_of_view").ReadOnly || field(t, p, "graphics.display_mode").ReadOnly {
		t.Error("FOV should be locked and window mode editable")
	}
}

func TestReclaimerSaveKeepsEverythingElse(t *testing.T) {
	dir, backups := setup(t, "settings.json", reclaimerFile)
	p := Load("reclaimer", dir)
	got, backup, err := Save("reclaimer", dir, backups, p.Stamp,
		map[string]string{"graphics.display_mode": "borderless", "audio.master_volume": "50", "graphics.vsync": "on"}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(`"display_mode": "windowed"`, `"display_mode": "borderless"`,
		`"master_volume": 1.0`, `"master_volume": 0.5`,
		// json.Indent puts array items on their own lines.
		`"a": [1, 2]`, "\"a\": [\n      1,\n      2\n    ]").Replace(reclaimerFile)
	if s := read(t, filepath.Join(dir, "settings.json")); s != want {
		t.Fatalf("file:\n%s\nwant:\n%s", s, want)
	}
	if read(t, backup) != reclaimerFile {
		t.Error("backup isn't the original file")
	}
	if field(t, got, "graphics.display_mode").Value != "borderless" {
		t.Error("reloaded page doesn't show the change")
	}
}

// The game's own file must come back byte for byte when nothing changes.
func TestReclaimerRoundTrip(t *testing.T) {
	real := reclaimerFile[:strings.Index(reclaimerFile, `,
  "future_section"`)] + "\n}"
	d, err := parseReclaimer([]byte(real))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(d.bytes()); got != real {
		t.Fatalf("round trip changed the file:\n%s", got)
	}
}

func TestReclaimerVolumeWholeKeepsDecimal(t *testing.T) {
	dir, backups := setup(t, "settings.json", strings.Replace(reclaimerFile, `"master_volume": 1.0`, `"master_volume": 0.5`, 1))
	p := Load("reclaimer", dir)
	if _, _, err := Save("reclaimer", dir, backups, p.Stamp, map[string]string{"audio.master_volume": "100"}, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "settings.json")), `"master_volume": 1.0`) {
		t.Error("volume should be written as 1.0, as the game writes floats")
	}
}

func TestSaveRefusals(t *testing.T) {
	dir, backups := setup(t, "settings.json", reclaimerFile)
	p := Load("reclaimer", dir)
	path := filepath.Join(dir, "settings.json")
	cases := map[string]map[string]string{
		"locked field":  {"field_of_view": "90"},
		"invalid value": {"graphics.display_mode": "fullscreen"},
		"unknown key":   {"graphics.nope": "x"},
	}
	for name, ch := range cases {
		if _, _, err := Save("reclaimer", dir, backups, p.Stamp, ch, now); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// The game saved after the page was loaded.
	os.WriteFile(path, []byte(strings.Replace(reclaimerFile, "1.5", "2.0", 1)), 0o644)
	if _, _, err := Save("reclaimer", dir, backups, p.Stamp, map[string]string{"graphics.vsync": "off"}, now); !errors.Is(err, ErrChanged) {
		t.Errorf("changed file: err = %v, want ErrChanged", err)
	}
	if !strings.Contains(read(t, path), `"vsync": true`) {
		t.Error("a refused save wrote to the file")
	}
	if _, err := os.Stat(backups); err == nil {
		t.Error("a refused save made a backup")
	}
}

func TestReclaimerUnknownFormat(t *testing.T) {
	for name, content := range map[string]string{
		"version 2":   strings.Replace(reclaimerFile, `"version": 1`, `"version": 2`, 1),
		"wrong type":  strings.Replace(reclaimerFile, `"vsync": true`, `"vsync": "yes"`, 1),
		"not json":    "fov 90",
		"duplicate":   strings.Replace(reclaimerFile, `"version": 1,`, `"version": 1, "version": 1,`, 1),
		"bad section": strings.Replace(reclaimerFile, `"graphics": {`, `"graphics": 5, "x": {`, 1),
	} {
		dir, _ := setup(t, "settings.json", content)
		if p := Load("reclaimer", dir); p.Error == "" {
			t.Errorf("%s: expected an error", name)
		}
	}
}

const reachFile = "display=borderless\nmusic=reach\nname=TS251\ntheme=title\n"

func TestReachLoadShowsDefaults(t *testing.T) {
	dir, _ := setup(t, "settings.txt", reachFile)
	p := Load("reach-renaissance", dir)
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	if field(t, p, "display").Value != "borderless" || field(t, p, "fov").Value != "78" || field(t, p, "gfx.preset").Value != "enhanced" {
		t.Errorf("unexpected values: %+v", p.Fields[:3])
	}
}

func TestReachSaveEditsInPlace(t *testing.T) {
	dir, backups := setup(t, "settings.txt", reachFile)
	p := Load("reach-renaissance", dir)
	_, backup, err := Save("reach-renaissance", dir, backups, p.Stamp,
		map[string]string{"display": "windowed", "fov": "90", "volume.game": "100"}, now)
	if err != nil {
		t.Fatal(err)
	}
	// display is changed in place, fov is added, volume.game stays out (default).
	want := "display=windowed\nmusic=reach\nname=TS251\ntheme=title\nfov=90\n"
	if s := read(t, filepath.Join(dir, "settings.txt")); s != want {
		t.Fatalf("file = %q, want %q", s, want)
	}
	if read(t, backup) != reachFile {
		t.Error("backup isn't the original file")
	}
}

func TestReachDefaultOnlyChangeWritesNothing(t *testing.T) {
	dir, backups := setup(t, "settings.txt", reachFile)
	p := Load("reach-renaissance", dir)
	_, backup, err := Save("reach-renaissance", dir, backups, p.Stamp, map[string]string{"fov": "78"}, now)
	if err != nil || backup != "" {
		t.Fatalf("err=%v backup=%q, want no write", err, backup)
	}
}

func TestReachGraphicsPreset(t *testing.T) {
	dir, backups := setup(t, "settings.txt", reachFile)
	path := filepath.Join(dir, "settings.txt")
	p := Load("reach-renaissance", dir)
	p, _, err := Save("reach-renaissance", dir, backups, p.Stamp, map[string]string{"gfx.preset": "performance"}, now)
	if err != nil {
		t.Fatal(err)
	}
	s := read(t, path)
	for _, l := range []string{"gfx.aa=off", "gfx.shadows=low", "gfx.motionblur=off", "gfx.preset=performance"} {
		if !strings.Contains(s, l+"\n") {
			t.Errorf("missing %q in:\n%s", l, s)
		}
	}
	// One row changed by hand makes the preset custom, as in the game's menu.
	p, _, err = Save("reach-renaissance", dir, backups, p.Stamp, map[string]string{"gfx.shadows": "high"}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if f := field(t, p, "gfx.preset"); f.Value != "custom" || !hasOption(f.Options, "custom") {
		t.Errorf("preset = %q, want custom", f.Value)
	}
	if !strings.Contains(read(t, path), "gfx.preset=custom\n") {
		t.Error("stored preset should be custom")
	}
}

func TestReachUnknownFormat(t *testing.T) {
	for name, content := range map[string]string{
		"no equals": "display=windowed\njust words\n",
		"duplicate": "fov=90\nfov=95\n",
	} {
		dir, _ := setup(t, "settings.txt", content)
		if p := Load("reach-renaissance", dir); p.Error == "" {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestReachCRLFKept(t *testing.T) {
	dir, backups := setup(t, "settings.txt", "display=borderless\r\nname=TS251\r\n")
	p := Load("reach-renaissance", dir)
	if _, _, err := Save("reach-renaissance", dir, backups, p.Stamp, map[string]string{"fps": "144"}, now); err != nil {
		t.Fatal(err)
	}
	if s := read(t, filepath.Join(dir, "settings.txt")); s != "display=borderless\r\nname=TS251\r\nfps=144\r\n" {
		t.Errorf("file = %q", s)
	}
}

func TestMissingFile(t *testing.T) {
	if p := Load("reach-renaissance", t.TempDir()); p.Error == "" || p.Fields != nil {
		t.Errorf("missing file should be an error: %+v", p)
	}
}

func TestBackupsArePruned(t *testing.T) {
	dir, backups := setup(t, "settings.txt", reachFile)
	fps := []string{"30", "60", "90", "120", "144", "165", "240"}
	for i := 0; i < MaxBackups+5; i++ {
		p := Load("reach-renaissance", dir)
		if _, _, err := Save("reach-renaissance", dir, backups, p.Stamp,
			map[string]string{"fps": fps[i%len(fps)]}, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := filepath.Glob(filepath.Join(backups, "settings.txt.*.bak"))
	if len(m) != MaxBackups {
		t.Errorf("%d backups kept, want %d", len(m), MaxBackups)
	}
}
