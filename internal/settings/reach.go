package settings

import (
	"fmt"
	"strconv"
	"strings"
)

// Reach Renaissance: %LOCALAPPDATA%\ReachRenaissance\settings.txt, one
// key=value per line, holding only the settings changed from the game's
// defaults. Keys, defaults and options follow the game's own menu
// (themes\classic\start-menu.js in its release).

type lineField struct {
	key     string
	label   string
	group   string
	def     string
	options []Option
}

// fovOptions is Reach Renaissance's field-of-view list; Project Reclaimer
// uses it too so the two games can share a value.
var fovOptions = same("", "70", "78", "85", "90", "95", "100", "110", "120")

var reachLevel = opts("low", "Low", "normal", "Normal", "high", "High")

var reachGamma = func() []Option {
	var out []Option
	for i := 0; i <= 18; i++ {
		v := 1 + float64(i)/2
		out = append(out, Option{Value: strconv.FormatFloat(v, 'f', -1, 64), Label: strconv.FormatFloat(v, 'f', 1, 64)})
	}
	return out
}()

var reachFields = []lineField{
	{key: "display", label: "Window mode", group: "Video", def: "windowed",
		options: opts("windowed", "Windowed", "borderless", "Borderless fullscreen")},
	{key: "resolution", label: "Resolution", group: "Video", def: "auto",
		options: append(opts("auto", "Automatic"), opts(
			"1280x720", "1280 x 720", "1600x900", "1600 x 900", "1920x1080", "1920 x 1080",
			"2560x1080", "2560 x 1080", "2560x1440", "2560 x 1440", "3440x1440", "3440 x 1440",
			"3840x2160", "3840 x 2160")...)},
	{key: "fov", label: "Field of view", group: "Video", def: "78", options: fovOptions},
	{key: "fov.vehicle", label: "Third-person field of view", group: "Video", def: "78", options: fovOptions},
	{key: "fps", label: "Framerate limit", group: "Video", def: "0",
		options: append(opts("0", "Unlimited"), same(" FPS", "30", "60", "90", "120", "144", "165", "240")...)},
	{key: "vsync", label: "V-sync", group: "Video", def: "off", options: onOff},
	{key: "gamma", label: "Gamma", group: "Video", def: "5", options: reachGamma},
	{key: "gfx.preset", label: "Graphics quality", group: "Graphics", def: "enhanced",
		options: opts("performance", "Performance", "original", "Original", "enhanced", "Enhanced")},
	{key: "gfx.aa", label: "Anti-aliasing", group: "Graphics", def: "on", options: onOff},
	{key: "gfx.details", label: "Details quality", group: "Graphics", def: "high", options: reachLevel},
	{key: "gfx.effects", label: "Effects quality", group: "Graphics", def: "high", options: reachLevel},
	{key: "gfx.lighting", label: "Lighting quality", group: "Graphics", def: "high", options: reachLevel},
	{key: "gfx.shadows", label: "Shadow quality", group: "Graphics", def: "high", options: reachLevel},
	{key: "gfx.filtering", label: "Anisotropic filtering", group: "Graphics", def: "high", options: reachLevel},
	{key: "gfx.water", label: "Water quality", group: "Graphics", def: "high", options: reachLevel},
	{key: "gfx.motionblur", label: "Motion blur", group: "Graphics", def: "on", options: onOff},
	{key: "mouse", label: "Mouse sensitivity", group: "Mouse", def: "100",
		options: same("", "50", "60", "70", "80", "90", "100", "110", "125", "150", "175", "200", "250", "300")},
	{key: "mouse.smoothing", label: "Mouse smoothing", group: "Mouse", def: "off", options: onOff},
	{key: "crosshair", label: "Crosshair position", group: "Mouse", def: "centered",
		options: opts("lowered", "Lowered", "centered", "Centered")},
	{key: "volume.game", label: "Game volume", group: "Audio", def: "100", options: percentOptions},
	{key: "volume.music", label: "Game music volume", group: "Audio", def: "100", options: percentOptions},
	{key: "volume.effects", label: "Effects volume", group: "Audio", def: "100", options: percentOptions},
	{key: "volume.menu", label: "Menu sounds volume", group: "Audio", def: "100", options: percentOptions},
	{key: "volume.menumusic", label: "Menu music volume", group: "Audio", def: "100", options: percentOptions},
	{key: "subtitles", label: "Subtitles", group: "Audio", def: "automatic",
		options: opts("automatic", "Automatic", "on", "On", "off", "Off")},
}

// The graphics rows a preset sets, and each preset's values (the game's
// GFX_PRESETS: every level row low, normal or high).
var reachGfxRows = []string{"gfx.aa", "gfx.details", "gfx.effects", "gfx.lighting", "gfx.shadows", "gfx.filtering", "gfx.water", "gfx.motionblur"}

func reachPresetValues(name string) map[string]string {
	level, onoff := map[string]string{"performance": "low", "original": "normal", "enhanced": "high"}[name], "on"
	if level == "" {
		return nil
	}
	if name == "performance" {
		onoff = "off"
	}
	m := map[string]string{}
	for _, k := range reachGfxRows {
		if k == "gfx.aa" || k == "gfx.motionblur" {
			m[k] = onoff
		} else {
			m[k] = level
		}
	}
	return m
}

var reachFormat = format{
	file:    "settings.txt",
	note:    "Values match Reach Renaissance's own settings menu. Settings left at the game's default aren't written to the file.",
	missing: "Reach Renaissance hasn't saved any settings yet. Run it once and change any setting in its menu.",
	parse:   parseReach,
	presets: map[string]map[string]map[string]string{"gfx.preset": {
		"performance": reachPresetValues("performance"),
		"original":    reachPresetValues("original"),
		"enhanced":    reachPresetValues("enhanced"),
	}},
}

type reachLine struct {
	key, value string // key "" for a blank line
	raw        string // the line as read, without its line ending
}

type reachDoc struct {
	lines []reachLine
	eol   string // "\n" or "\r\n", as the file uses
	nl    bool   // the file ended with a line ending
}

func parseReach(data []byte) (doc, error) {
	s := string(data)
	d := &reachDoc{eol: "\n"}
	if strings.Contains(s, "\r\n") {
		d.eol = "\r\n"
	}
	d.nl = strings.HasSuffix(s, "\n")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\r")
	if s == "" {
		return d, nil
	}
	seen := map[string]bool{}
	for i, l := range strings.Split(s, "\n") {
		l = strings.TrimSuffix(l, "\r")
		if strings.TrimSpace(l) == "" {
			d.lines = append(d.lines, reachLine{raw: l})
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("line %d isn't key=value", i+1)
		}
		if seen[k] {
			return nil, fmt.Errorf("%q appears twice", k)
		}
		seen[k] = true
		d.lines = append(d.lines, reachLine{key: k, value: strings.TrimSpace(v), raw: l})
	}
	return d, nil
}

func (d *reachDoc) value(key string) (string, bool) {
	for _, l := range d.lines {
		if l.key == key {
			return l.value, true
		}
	}
	return "", false
}

func (d *reachDoc) current(f lineField) string {
	if v, ok := d.value(f.key); ok {
		return v
	}
	return f.def
}

func (d *reachDoc) fields() []Field {
	out := make([]Field, 0, len(reachFields))
	for _, f := range reachFields {
		cur := d.current(f)
		o := f.options
		if f.key == "gfx.preset" && d.preset() == "custom" {
			cur = "custom"
			o = append(append([]Option{}, o...), Option{Value: "custom", Label: "Custom"})
		}
		out = append(out, Field{Key: f.key, Label: f.label, Group: f.group, Value: cur, Options: withCurrent(o, cur)})
	}
	return out
}

// preset is the preset the graphics rows add up to, or "custom".
func (d *reachDoc) preset() string {
	for _, name := range []string{"performance", "original", "enhanced"} {
		vals, match := reachPresetValues(name), true
		for _, k := range reachGfxRows {
			if d.current(reachField(k)) != vals[k] {
				match = false
				break
			}
		}
		if match {
			return name
		}
	}
	return "custom"
}

func reachField(key string) lineField {
	for _, f := range reachFields {
		if f.key == key {
			return f
		}
	}
	return lineField{key: key}
}

func (d *reachDoc) apply(changes map[string]string) error {
	// A preset sets every graphics row first; rows chosen alongside it win.
	if p, ok := changes["gfx.preset"]; ok && p != "custom" {
		for k, v := range reachPresetValues(p) {
			d.set(k, v)
		}
	}
	for _, f := range reachFields {
		if v, ok := changes[f.key]; ok && f.key != "gfx.preset" {
			d.set(f.key, v)
		}
	}
	// Like the game's menu: the stored preset is what the rows add up to.
	if p := d.preset(); p != d.current(reachField("gfx.preset")) {
		d.set("gfx.preset", p)
	}
	return nil
}

// set changes a key's line in place. A key that isn't in the file is only
// added when the value differs from the game's default.
func (d *reachDoc) set(key, value string) {
	for i, l := range d.lines {
		if l.key == key {
			if l.value != value {
				d.lines[i] = reachLine{key: key, value: value, raw: key + "=" + value}
			}
			return
		}
	}
	if value == reachField(key).def {
		return
	}
	d.lines = append(d.lines, reachLine{key: key, value: value, raw: key + "=" + value})
	d.nl = true // the game ends every line, so a new last line is ended too
}

func (d *reachDoc) bytes() []byte {
	var b strings.Builder
	for i, l := range d.lines {
		if i > 0 {
			b.WriteString(d.eol)
		}
		b.WriteString(l.raw)
	}
	if len(d.lines) > 0 && d.nl {
		b.WriteString(d.eol)
	}
	return []byte(b.String())
}
