package proc

import (
	"path/filepath"
	"testing"
)

func TestUnderDir(t *testing.T) {
	d := filepath.Join("games", "reach")
	if !underDir(filepath.Join(d, "Reach Renaissance.exe"), d) {
		t.Error("should match")
	}
	if underDir(filepath.Join("games", "reach2", "x.exe"), d) {
		t.Error("sibling prefix must not match")
	}
}
