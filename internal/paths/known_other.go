//go:build !windows

package paths

import (
	"os"
	"path/filepath"
)

// Off Windows these only exist for tests; RECLAIMED_FAKE_PROFILE points at a fake
// user profile.
func Documents() string { return filepath.Join(os.Getenv("RECLAIMED_FAKE_PROFILE"), "Documents") }
func LocalAppData() string {
	return filepath.Join(os.Getenv("RECLAIMED_FAKE_PROFILE"), "AppData", "Local")
}
func Desktop() string { return filepath.Join(os.Getenv("RECLAIMED_FAKE_PROFILE"), "Desktop") }
