package settings

import (
	"path/filepath"
	"strings"
	"testing"
)

func loadBoth(t *testing.T, reclaimer, reach string) (rp, ap Page, rDir, aDir, backups string) {
	t.Helper()
	rDir, backups = setup(t, "settings.json", reclaimer)
	aDir, _ = setup(t, "settings.txt", reach)
	rp, ap = Load("reclaimer", rDir), Load("reach-renaissance", aDir)
	if rp.Error != "" || ap.Error != "" {
		t.Fatal(rp.Error, ap.Error)
	}
	return
}

func row(t *testing.T, rows []SyncRow, key string) SyncRow {
	t.Helper()
	for _, r := range rows {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("no row %q", key)
	return SyncRow{}
}

func TestCompare(t *testing.T) {
	// Reach: fov 90, fps unlimited (default 0), v-sync off (default).
	rp, ap, _, _, _ := loadBoth(t, reclaimerFile, "display=borderless\nfov=90\n")
	rows := Compare(rp, ap)
	if len(rows) != len(Links) {
		t.Fatalf("%d rows, want %d", len(rows), len(Links))
	}
	fov := row(t, rows, "fov")
	if fov.Same || fov.Sides["reclaimer"].Value != "85" || fov.Sides["reach-renaissance"].Value != "90" {
		t.Errorf("fov row: %+v", fov)
	}
	if !row(t, rows, "volume").Same {
		t.Error("both volumes are 100%")
	}
	fps := row(t, rows, "fps")
	if fps.Sides["reach-renaissance"].CanCopy || fps.Sides["reach-renaissance"].Note == "" {
		t.Error("Reach's unlimited frame rate has no Reclaimer value")
	}
	if !fps.Sides["reclaimer"].CanCopy || fps.Sides["reclaimer"].Label != "120 FPS" {
		t.Errorf("Reclaimer's 120 FPS should copy to Reach: %+v", fps.Sides["reclaimer"])
	}
}

func TestCopyAllReclaimerToReach(t *testing.T) {
	rp, ap, _, aDir, backups := loadBoth(t, reclaimerFile, "display=borderless\nfov=90\nname=TS251\n")
	ch, err := CopyChanges(rp, ap, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"fov": "85", "fov.vehicle": "70", "fps": "120", "vsync": "on", "display": "windowed"}
	if len(ch) != len(want) {
		t.Fatalf("changes = %v, want %v", ch, want)
	}
	for k, v := range want {
		if ch[k] != v {
			t.Errorf("%s = %q, want %q", k, ch[k], v)
		}
	}
	got, _, err := Save("reach-renaissance", aDir, backups, ap.Stamp, ch, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range Compare(rp, got) {
		if !r.Same {
			t.Errorf("%s still differs after copying everything", r.Key)
		}
	}
	if s := read(t, filepath.Join(aDir, "settings.txt")); !strings.Contains(s, "name=TS251\n") {
		t.Error("an unrelated line was lost")
	}
}

func TestCopySkipsWhatTheOtherGameLacks(t *testing.T) {
	reclaimer := strings.Replace(reclaimerFile, `"display_mode": "windowed"`, `"display_mode": "exclusive_fullscreen"`, 1)
	rp, ap, _, _, _ := loadBoth(t, reclaimer, "fps=0\n")
	// Copying everything quietly leaves out exclusive fullscreen...
	ch, err := CopyChanges(rp, ap, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ch["display"]; ok {
		t.Error("exclusive fullscreen has no Reach value")
	}
	// ...but asking for it by name is an error.
	if _, err := CopyChanges(rp, ap, []string{"display"}); err == nil {
		t.Error("expected an error copying exclusive fullscreen to Reach")
	}
	// The other way, unlimited (0) isn't a Reclaimer value.
	ch, err = CopyChanges(ap, rp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ch["graphics.frame_limit"]; ok {
		t.Error("Reach's unlimited frame rate shouldn't be copied")
	}
	if ch["field_of_view"] != "78" {
		t.Errorf("Reach's default FOV should copy: %v", ch)
	}
}

func TestCopyOneSetting(t *testing.T) {
	rp, ap, _, _, _ := loadBoth(t, reclaimerFile, "fov=90\n")
	ch, err := CopyChanges(ap, rp, []string{"fov"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 1 || ch["field_of_view"] != "90" {
		t.Errorf("changes = %v", ch)
	}
	if _, err := CopyChanges(ap, rp, []string{"mouse"}); err == nil {
		t.Error("expected an error for a setting that isn't shared")
	}
}
