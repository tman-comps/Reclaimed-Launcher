package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateShortcut(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	lnk := filepath.Join(dir, "it's a test", "Reclaimed.lnk") // a quote must not break the script
	os.MkdirAll(filepath.Dir(lnk), 0o755)
	if err := CreateShortcut(lnk, target, dir, "test"); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(lnk); err != nil || st.Size() == 0 {
		t.Fatal(err)
	}
}
