// Package games knows the two supported clients: where their releases live,
// how they are packaged, and how to tell which version is on disk.
package games

import (
	"path/filepath"
	"regexp"

	"reclaimed/internal/paths"
	"reclaimed/internal/release"
)

// Kind is how a release is packaged.
type Kind int

const (
	// KindExe is a single executable (Project Reclaimer).
	KindExe Kind = iota
	// KindZip is a folder shipped as a zip (Reach Renaissance).
	KindZip
)

// Def describes one game client.
type Def struct {
	ID        string
	Name      string
	Tagline   string
	Repo      string // owner/name on GitHub
	Needs     string // MCC component id, see steam.Component
	NeedsName string
	Kind      Kind
	// Assets are tried in order; earlier patterns are newer naming schemes.
	Assets []release.Pattern
	// ExeName is the fixed exe inside a KindZip folder.
	ExeName string
	// ExePatterns find the client exe inside a KindExe folder.
	ExePatterns []*regexp.Regexp
	// MustNotBeInsideMCC: the game refuses to run from inside the MCC folder.
	MustNotBeInsideMCC bool
	// UserData returns the game's own settings/log folder (read-only for us).
	UserData func() string
	// SelfUpdateNote explains the game's built-in updater in the UI.
	SelfUpdateNote string
}

var dedicated = regexp.MustCompile(`(?i)dedicated`)

// Reclaimer client naming, newest first (see the releases README):
// project-reclaimer-client-v<ver>.exe, project-reclaimer-v<ver>.exe,
// project-reclaimer.exe. The dedicated server exe can't start the game.
var reclaimerExes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^project-reclaimer-client-v?(\d[0-9A-Za-z.\-+]*)\.exe$`),
	regexp.MustCompile(`(?i)^project-reclaimer-v?(\d[0-9A-Za-z.\-+]*)\.exe$`),
	regexp.MustCompile(`(?i)^project-reclaimer\.exe$`),
}

// All is every supported game, in display order.
var All = []*Def{
	{
		ID:        "reclaimer",
		Name:      "Project Reclaimer",
		Tagline:   "Halo 3 multiplayer client",
		Repo:      "ProjectReclaimer/project-reclaimer-releases",
		Needs:     "halo3",
		NeedsName: "Halo 3",
		Kind:      KindExe,
		Assets: []release.Pattern{
			{Re: reclaimerExes[0], Exclude: dedicated},
			{Re: reclaimerExes[1], Exclude: dedicated},
			{Re: reclaimerExes[2], Exclude: dedicated},
		},
		ExePatterns: reclaimerExes,
		UserData: func() string {
			return filepath.Join(paths.Documents(), "My Games", "Project Reclaimer")
		},
		SelfUpdateNote: "Updates itself from its in-game menu by replacing its exe. The launcher recognises those updates by checksum.",
	},
	{
		ID:        "reach-renaissance",
		Name:      "Reach Renaissance",
		Tagline:   "Halo: Reach multiplayer, Firefight & Campaign",
		Repo:      "NETF1ix/reach-renaissance-releases",
		Needs:     "haloreach",
		NeedsName: "Halo: Reach",
		Kind:      KindZip,
		Assets: []release.Pattern{
			{Re: regexp.MustCompile(`(?i)^ReachRenaissance-v?(\d[0-9A-Za-z.\-+]*)\.zip$`)},
		},
		ExeName:            "Reach Renaissance.exe",
		MustNotBeInsideMCC: true,
		UserData: func() string {
			return filepath.Join(paths.LocalAppData(), "ReachRenaissance")
		},
		SelfUpdateNote: "Offers signed updates when it starts (Update and restart). The launcher steps aside while it runs.",
	},
}

// ByID finds a game definition.
func ByID(id string) *Def {
	for _, d := range All {
		if d.ID == id {
			return d
		}
	}
	return nil
}
