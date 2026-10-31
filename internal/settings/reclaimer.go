package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Project Reclaimer: Documents\My Games\Project Reclaimer\settings.json,
// pretty-printed JSON with "version": 1. Values for the enums come from the
// game exe's own text. The game doesn't name its number ranges, so field of
// view and frame limit use lists that hold for any Halo game (the same FOV
// list as Reach Renaissance, so the two can sync).

type kind int

const (
	kindString kind = iota
	kindBool
	kindNumber // shown only
	kindInt
	kindFloat  // written with a ".0" when whole, as the game writes floats
	kindVolume // 0..1 in the file, percent in the UI
)

type jsonField struct {
	path    string // dotted: "graphics.vsync"
	label   string
	group   string
	kind    kind
	options []Option
	locked  string // why it can't be changed yet ("" = editable)
}

var percentOptions = same("%", "0", "10", "20", "30", "40", "50", "60", "70", "80", "90", "100")

var onOff = opts("off", "Off", "on", "On")

var reclaimerFields = []jsonField{
	{path: "graphics.display_mode", label: "Window mode", group: "Video", kind: kindString,
		options: opts("windowed", "Windowed", "borderless", "Borderless", "exclusive_fullscreen", "Exclusive fullscreen")},
	{path: "graphics.render_resolution", label: "Render resolution", group: "Video", kind: kindString,
		options: opts("r720p", "720p", "r900p", "900p", "r1080p", "1080p", "r1440p", "1440p", "r2160p", "2160p (4K)")},
	{path: "graphics.vsync", label: "V-sync", group: "Video", kind: kindBool},
	// No "unlimited": how the game stores it isn't known.
	{path: "graphics.frame_limit", label: "Frame limit", group: "Video", kind: kindInt,
		options: same(" FPS", "30", "60", "90", "120", "144", "165", "240")},
	{path: "graphics.width", label: "Window width", group: "Video", kind: kindNumber, locked: "Set by resizing the game's window."},
	{path: "graphics.height", label: "Window height", group: "Video", kind: kindNumber, locked: "Set by resizing the game's window."},
	{path: "field_of_view", label: "Field of view", group: "Video", kind: kindFloat, options: fovOptions},
	{path: "vehicle_field_of_view", label: "Vehicle field of view", group: "Video", kind: kindFloat, options: fovOptions},
	{path: "audio.master_volume", label: "Master volume", group: "Audio", kind: kindVolume, options: percentOptions},
	{path: "audio.muted", label: "Mute all sound", group: "Audio", kind: kindBool},
	{path: "audio.mute_unfocused", label: "Mute in the background", group: "Audio", kind: kindBool},
	{path: "audio.main_menu_music", label: "Main menu music", group: "Audio", kind: kindBool},
}

var reclaimerFormat = format{
	file:     "settings.json",
	note:     "Settings not shown here (mouse, keybinds and others) are left exactly as they are.",
	missing:  "Project Reclaimer hasn't saved its settings yet. Run it once and change any setting in its menu.",
	parse:    parseReclaimer,
	firstRun: firstRun{key: "graphics.display_mode", value: "borderless", gameDefault: "windowed"},
}

type reclaimerDoc struct {
	root *object
	nl   bool // the file ended with a newline
}

func parseReclaimer(data []byte) (doc, error) {
	root, err := parseObject(data)
	if err != nil {
		return nil, err
	}
	v, ok := root.vals["version"]
	if !ok || strings.TrimSpace(string(v)) != "1" {
		return nil, fmt.Errorf("expected \"version\": 1, found %s", strings.TrimSpace(string(v)))
	}
	d := &reclaimerDoc{root: root, nl: bytes.HasSuffix(data, []byte("\n"))}
	// Every known field that is present must have the expected type.
	for _, f := range reclaimerFields {
		raw, ok, err := d.get(f.path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if _, err := decode(f, raw); err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
	}
	return d, nil
}

// get returns the raw value at a dotted path.
func (d *reclaimerDoc) get(path string) (json.RawMessage, bool, error) {
	obj := d.root
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		raw, ok := obj.vals[p]
		if !ok {
			return nil, false, nil
		}
		sub, err := parseObject(raw)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", p, err)
		}
		obj = sub
	}
	raw, ok := obj.vals[parts[len(parts)-1]]
	return raw, ok, nil
}

// set replaces the raw value at an existing dotted path.
func (d *reclaimerDoc) set(path string, val json.RawMessage) error {
	parts := strings.Split(path, ".")
	return setIn(d.root, parts, val)
}

func setIn(obj *object, parts []string, val json.RawMessage) error {
	raw, ok := obj.vals[parts[0]]
	if !ok {
		return fmt.Errorf("%s isn't in the file", parts[0])
	}
	if len(parts) == 1 {
		obj.vals[parts[0]] = val
		return nil
	}
	sub, err := parseObject(raw)
	if err != nil {
		return err
	}
	if err := setIn(sub, parts[1:], val); err != nil {
		return err
	}
	obj.vals[parts[0]] = sub.compact()
	return nil
}

func (d *reclaimerDoc) fields() []Field {
	out := make([]Field, 0, len(reclaimerFields))
	for _, f := range reclaimerFields {
		fld := Field{Key: f.path, Label: f.label, Group: f.group}
		raw, ok, _ := d.get(f.path)
		if !ok {
			fld.ReadOnly = true
			fld.Note = "Not in the file yet; the game adds it when you change it there."
			out = append(out, fld)
			continue
		}
		fld.Value, _ = decode(f, raw)
		switch {
		case f.locked != "":
			fld.ReadOnly, fld.Note = true, f.locked
		case f.kind == kindBool:
			fld.Options = onOff
		default:
			fld.Options = withCurrent(f.options, fld.Value)
		}
		out = append(out, fld)
	}
	return out
}

func (d *reclaimerDoc) apply(changes map[string]string) error {
	for _, f := range reclaimerFields {
		v, ok := changes[f.path]
		if !ok {
			continue
		}
		raw, _, _ := d.get(f.path)
		if cur, _ := decode(f, raw); cur == v {
			continue // keep the game's own spelling of an unchanged value
		}
		enc, err := encode(f, v)
		if err != nil {
			return fmt.Errorf("%s: %w", f.label, err)
		}
		if err := d.set(f.path, enc); err != nil {
			return err
		}
	}
	return nil
}

func (d *reclaimerDoc) bytes() []byte {
	var out bytes.Buffer
	json.Indent(&out, d.root.compact(), "", "  ")
	if d.nl {
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// decode turns a raw JSON value into the UI's string form.
func decode(f jsonField, raw json.RawMessage) (string, error) {
	switch f.kind {
	case kindString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", errors.New("expected text")
		}
		return s, nil
	case kindBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", errors.New("expected true or false")
		}
		if b {
			return "on", nil
		}
		return "off", nil
	default:
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return "", errors.New("expected a number")
		}
		if f.kind == kindVolume {
			if n < 0 || n > 1 {
				return "", fmt.Errorf("volume %v is outside 0 to 1", n)
			}
			return strconv.Itoa(int(math.Round(n * 100))), nil
		}
		return trimNumber(strings.TrimSpace(string(raw))), nil
	}
}

// encode turns a UI value back into JSON in the game's own style (floats
// keep a ".0", as the game writes them).
func encode(f jsonField, v string) (json.RawMessage, error) {
	switch f.kind {
	case kindString:
		return json.Marshal(v)
	case kindBool:
		return json.RawMessage(strconv.FormatBool(v == "on")), nil
	case kindInt:
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", v)
		}
		return json.RawMessage(strconv.Itoa(n)), nil
	case kindFloat:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", v)
		}
		return json.RawMessage(floatJSON(n)), nil
	case kindVolume:
		pct, err := strconv.Atoi(v)
		if err != nil || pct < 0 || pct > 100 {
			return nil, fmt.Errorf("bad volume %q", v)
		}
		return json.RawMessage(floatJSON(float64(pct) / 100)), nil
	}
	return nil, errors.New("this setting can't be changed here")
}

// floatJSON writes a float the way the game does: 90.0, 0.5.
func floatJSON(n float64) string {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// object is a JSON object that keeps its keys in file order.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseObject(data []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, errors.New("expected a JSON object")
	}
	o := &object{vals: map[string]json.RawMessage{}}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k, _ := t.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if _, dup := o.vals[k]; dup {
			return nil, fmt.Errorf("key %q appears twice", k)
		}
		o.keys = append(o.keys, k)
		o.vals[k] = v
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON object")
	}
	return o, nil
}

// compact encodes the object in key order without whitespace.
func (o *object) compact() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		json.Compact(&b, o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes()
}
