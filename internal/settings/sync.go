package settings

import (
	"errors"
	"fmt"
)

// Link is a setting both games have (see "Sync between the games" in
// docs/settings-formats.md). Both pages show these fields with the same
// value spelling, so a value copies across unchanged when the other game
// offers it.
type Link struct {
	Key    string
	Label  string
	Fields map[string]string // game id -> field key
}

// Links is every setting the launcher can copy between the games.
var Links = []Link{
	{Key: "fov", Label: "Field of view",
		Fields: map[string]string{"reclaimer": "field_of_view", "reach-renaissance": "fov"}},
	{Key: "fov.vehicle", Label: "Vehicle / third-person field of view",
		Fields: map[string]string{"reclaimer": "vehicle_field_of_view", "reach-renaissance": "fov.vehicle"}},
	{Key: "fps", Label: "Frame limit",
		Fields: map[string]string{"reclaimer": "graphics.frame_limit", "reach-renaissance": "fps"}},
	{Key: "vsync", Label: "V-sync",
		Fields: map[string]string{"reclaimer": "graphics.vsync", "reach-renaissance": "vsync"}},
	{Key: "display", Label: "Window mode",
		Fields: map[string]string{"reclaimer": "graphics.display_mode", "reach-renaissance": "display"}},
	{Key: "volume", Label: "Volume",
		Fields: map[string]string{"reclaimer": "audio.master_volume", "reach-renaissance": "volume.game"}},
}

// SyncSide is one game's side of a SyncRow.
type SyncSide struct {
	Value string `json:"value"`
	Label string `json:"label"` // the value as that game's page shows it
	// CanCopy: the other game offers this value, so it can be copied there.
	CanCopy bool   `json:"canCopy"`
	Note    string `json:"note,omitempty"` // why it can't be copied
}

// SyncRow compares one Link across the two games.
type SyncRow struct {
	Key   string              `json:"key"`
	Label string              `json:"label"`
	Same  bool                `json:"same"`
	Sides map[string]SyncSide `json:"sides"` // game id -> side
}

func pageField(p Page, key string) (Field, bool) {
	for _, f := range p.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

func optionLabel(f Field, v string) string {
	for _, o := range f.Options {
		if o.Value == v {
			return o.Label
		}
	}
	return v
}

// canTake reports whether field f can be set to v from the launcher.
func canTake(f Field, v string) bool { return !f.ReadOnly && hasOption(f.Options, v) }

// Compare lines up the shared settings of two loaded pages. Links missing
// from either page are left out.
func Compare(a, b Page) []SyncRow {
	var rows []SyncRow
	for _, l := range Links {
		fa, okA := pageField(a, l.Fields[a.Game])
		fb, okB := pageField(b, l.Fields[b.Game])
		if !okA || !okB {
			continue
		}
		row := SyncRow{Key: l.Key, Label: l.Label, Same: fa.Value == fb.Value, Sides: map[string]SyncSide{}}
		for _, s := range []struct {
			game     string
			from, to Field
			toName   string
		}{{a.Game, fa, fb, b.Game}, {b.Game, fb, fa, a.Game}} {
			side := SyncSide{Value: s.from.Value, Label: optionLabel(s.from, s.from.Value), CanCopy: canTake(s.to, s.from.Value)}
			if !side.CanCopy {
				side.Note = "the other game has no matching setting"
				if s.to.ReadOnly {
					side.Note = "the other game's setting can't be changed here"
				}
			}
			row.Sides[s.game] = side
		}
		rows = append(rows, row)
	}
	return rows
}

// CopyChanges builds the changes that copy the given links (all copyable
// differences when keys is empty) from one page to another, for Save on
// the to page.
func CopyChanges(from, to Page, keys []string) (map[string]string, error) {
	if from.Error != "" || to.Error != "" {
		return nil, errors.New("both games' settings must be readable to copy between them")
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	changes := map[string]string{}
	for _, r := range Compare(from, to) {
		if len(keys) > 0 && !want[r.Key] {
			continue
		}
		delete(want, r.Key)
		if r.Same {
			continue
		}
		src := r.Sides[from.Game]
		if !src.CanCopy {
			if len(keys) > 0 {
				return nil, fmt.Errorf("%s: %s can't be copied (%s)", r.Label, src.Label, src.Note)
			}
			continue
		}
		for _, l := range Links {
			if l.Key == r.Key {
				changes[l.Fields[to.Game]] = src.Value
			}
		}
	}
	for k := range want {
		return nil, fmt.Errorf("unknown shared setting %q", k)
	}
	return changes, nil
}
