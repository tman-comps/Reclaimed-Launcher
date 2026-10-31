// Package settings reads and edits each game's own settings file
// (Phase 2). Formats are described in docs/settings-formats.md.
//
// Every write follows the owner's rules: the file must still be exactly what
// was loaded, its format must be recognised, the old copy is backed up into
// the launcher's own folder first, and the new file replaces it atomically.
// Keys and lines the launcher doesn't know are kept as they were. Refusing
// while the game runs is the caller's job (it knows the game folder).
package settings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Option is one choice for a field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field is one setting as the UI shows it. Values are always strings.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Group    string   `json:"group"`
	Value    string   `json:"value"`
	Options  []Option `json:"options,omitempty"`
	ReadOnly bool     `json:"readOnly,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// Page is one game's settings file.
type Page struct {
	Game   string  `json:"game"`
	Path   string  `json:"path"`
	Stamp  string  `json:"stamp"` // SHA-256 of the file as loaded
	Fields []Field `json:"fields"`
	Note   string  `json:"note,omitempty"`
	Error  string  `json:"error,omitempty"` // set when the page can't be edited
	// Presets maps a preset field's value to the values it sets on other
	// fields, so the UI can show them before saving.
	Presets map[string]map[string]map[string]string `json:"presets,omitempty"`
}

// doc is a parsed settings file.
type doc interface {
	fields() []Field
	// apply sets already-validated values.
	apply(changes map[string]string) error
	bytes() []byte
}

type format struct {
	file  string
	note  string
	parse func([]byte) (doc, error)
	// missing explains what to do when the file doesn't exist yet.
	missing string
	presets map[string]map[string]map[string]string
	// firstRun is the setting the launcher changes once, before it first
	// starts the game (ApplyFirstRun).
	firstRun firstRun
}

// firstRun sets key to value when it still has the game's own default.
type firstRun struct {
	key, value, gameDefault string
	// create allows writing a new file when the game hasn't made one yet.
	create bool
}

var formats = map[string]format{
	"reclaimer":         reclaimerFormat,
	"reach-renaissance": reachFormat,
}

// MaxBackups is how many backups are kept per game.
const MaxBackups = 20

// ErrChanged means the file changed after it was loaded.
var ErrChanged = errors.New("the settings file changed since it was loaded (the game may have saved it); nothing was written. Reload and try again")

// Supported reports whether the launcher can edit this game's settings.
func Supported(id string) bool { _, ok := formats[id]; return ok }

// Load reads a game's settings file from its user-data folder.
func Load(id, dir string) Page {
	f, ok := formats[id]
	if !ok {
		return Page{Game: id, Error: "no settings support for this game"}
	}
	p := Page{Game: id, Path: filepath.Join(dir, f.file), Note: f.note, Presets: f.presets}
	data, err := os.ReadFile(p.Path)
	if errors.Is(err, os.ErrNotExist) {
		p.Error = f.missing
		return p
	}
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.Stamp = stamp(data)
	d, err := f.parse(data)
	if err != nil {
		p.Error = "unrecognised settings format, so the launcher won't edit it: " + err.Error()
		return p
	}
	p.Fields = d.fields()
	return p
}

// Save applies changes (key -> value) to the file at dir, if it still
// matches the loaded page's stamp. The previous file is copied to backupDir
// first. It returns the reloaded page and the backup's path ("" when nothing
// changed).
func Save(id, dir, backupDir, loaded string, changes map[string]string, now time.Time) (Page, string, error) {
	f, ok := formats[id]
	if !ok {
		return Page{}, "", errors.New("no settings support for this game")
	}
	path := filepath.Join(dir, f.file)
	data, err := os.ReadFile(path)
	if err != nil {
		return Load(id, dir), "", err
	}
	if stamp(data) != loaded {
		return Load(id, dir), "", ErrChanged
	}
	d, err := f.parse(data)
	if err != nil {
		return Load(id, dir), "", fmt.Errorf("unrecognised settings format: %w", err)
	}
	if err := validate(d.fields(), changes); err != nil {
		return Load(id, dir), "", err
	}
	if err := d.apply(changes); err != nil {
		return Load(id, dir), "", err
	}
	out := d.bytes()
	if bytes.Equal(out, data) {
		return Load(id, dir), "", nil
	}
	if _, err := f.parse(out); err != nil {
		return Load(id, dir), "", fmt.Errorf("internal error: the new file wouldn't be readable (%v); nothing was written", err)
	}
	backup, err := writeBackup(backupDir, f.file, data, now)
	if err != nil {
		return Load(id, dir), "", fmt.Errorf("couldn't back up the settings file, so nothing was written: %w", err)
	}
	if err := replaceFile(path, out); err != nil {
		return Load(id, dir), backup, err
	}
	pruneBackups(backupDir, f.file, MaxBackups)
	return Load(id, dir), backup, nil
}

// ApplyFirstRun makes borderless fullscreen the window mode before the
// launcher first starts a game (the owner's choice for new players). It only
// replaces the game's own default (windowed), so a choice the player made in
// the game is kept, and it backs the file up first like Save. done reports
// that it needn't be tried again: it's false only while Project Reclaimer
// hasn't written its settings file yet, which the launcher never creates
// (the game rejects files it doesn't recognise as its own).
func ApplyFirstRun(id, dir, backupDir string, now time.Time) (done bool, err error) {
	f, ok := formats[id]
	if !ok || f.firstRun.key == "" {
		return true, nil
	}
	fr := f.firstRun
	path := filepath.Join(dir, f.file)
	data, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	switch {
	case missing && !fr.create:
		return false, nil
	case err != nil && !missing:
		return false, err
	}
	d, err := f.parse(data)
	if err != nil {
		return true, fmt.Errorf("unrecognised settings format, left alone: %w", err)
	}
	var cur *Field
	for _, fld := range d.fields() {
		if fld.Key == fr.key {
			cur = &fld
			break
		}
	}
	if cur == nil || cur.ReadOnly || cur.Value != fr.gameDefault || !hasOption(cur.Options, fr.value) {
		return true, nil // the player's own choice, or a file the launcher can't change
	}
	if err := d.apply(map[string]string{fr.key: fr.value}); err != nil {
		return true, err
	}
	out := d.bytes()
	if _, err := f.parse(out); err != nil {
		return true, fmt.Errorf("internal error: the new file wouldn't be readable (%v); nothing was written", err)
	}
	if missing {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, err
		}
	} else {
		if _, err := writeBackup(backupDir, f.file, data, now); err != nil {
			return false, fmt.Errorf("couldn't back up the settings file, so nothing was written: %w", err)
		}
		pruneBackups(backupDir, f.file, MaxBackups)
	}
	return true, replaceFile(path, out)
}

func stamp(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func validate(fields []Field, changes map[string]string) error {
	byKey := map[string]Field{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	for k, v := range changes {
		f, ok := byKey[k]
		if !ok {
			return fmt.Errorf("unknown setting %q", k)
		}
		if f.ReadOnly {
			return fmt.Errorf("%s can't be changed here yet", f.Label)
		}
		if !hasOption(f.Options, v) {
			return fmt.Errorf("%q isn't a valid value for %s", v, f.Label)
		}
	}
	return nil
}

func hasOption(opts []Option, v string) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
}

// withCurrent adds the file's current value to opts when it isn't one of
// them, so an unexpected value is shown (and kept) rather than lost.
func withCurrent(opts []Option, cur string) []Option {
	if cur == "" || hasOption(opts, cur) {
		return opts
	}
	return append([]Option{{Value: cur, Label: cur + " (current)"}}, opts...)
}

func opts(pairs ...string) []Option {
	out := make([]Option, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Option{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

// same returns options whose label is the value, with an optional suffix.
func same(suffix string, values ...string) []Option {
	out := make([]Option, len(values))
	for i, v := range values {
		out[i] = Option{Value: v, Label: v + suffix}
	}
	return out
}

func writeBackup(dir, name string, data []byte, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name+"."+now.Format("20060102-150405")+".bak")
	for i := 2; fileExists(p); i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s.%s-%d.bak", name, now.Format("20060102-150405"), i))
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		os.Remove(p)
		return "", err
	}
	return p, nil
}

func pruneBackups(dir, name string, keep int) {
	m, _ := filepath.Glob(filepath.Join(dir, name+".*.bak"))
	if len(m) <= keep {
		return
	}
	sort.Strings(m) // timestamps sort by age
	for _, p := range m[:len(m)-keep] {
		os.Remove(p)
	}
}

// replaceFile writes data next to path and renames it over path, so the
// game never sees a half-written file.
func replaceFile(path string, data []byte) error {
	tmp := path + ".reclaimed-tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("couldn't write the settings file (the backup is kept): %w", err)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func trimNumber(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}
