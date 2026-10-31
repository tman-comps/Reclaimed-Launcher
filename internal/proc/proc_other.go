//go:build !windows

package proc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// processImages on non-Windows hosts reads /proc; it exists so the rest of
// the code can be tested off Windows.
func processImages() ([]string, error) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, nil
	}
	var out []string
	for _, e := range ents {
		if p, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func Launch(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func FileVersion(string) string { return "" }

func OpenFolder(string) error { return errors.New("not supported on this platform") }

func CreateShortcut(lnk, target, workDir, description string) error {
	return errors.New("not supported on this platform")
}

func WaitExit(pid int, timeout time.Duration) {}
