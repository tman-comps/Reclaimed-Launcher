//go:build !windows

package steam

import "os"

// SteamRoot on non-Windows hosts is only used by tests; RECLAIMED_STEAM_ROOT
// points at a fake Steam tree.
func SteamRoot() string { return os.Getenv("RECLAIMED_STEAM_ROOT") }
