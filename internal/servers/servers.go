// Package servers is the home for Phase 3: a combined server browser with
// join-from-launcher. It is deliberately inert.
//
// Blocked on the game developers documenting:
//   - a server-info query (name, map, mode, players) for each game, and
//   - a supported way to join a server (command-line argument, URI scheme,
//     or IPC), so the launcher never has to touch game memory or files.
//
// Today Project Reclaimer's discovery endpoint (see package discovery) only
// returns signed addresses, and Reach Renaissance lists servers in-game only.
//
// To implement: add a Source per game in Sources(), a Joiner per game, flip
// Available() to true, and the UI's Servers page (frontend: renderServers)
// will pick them up.
package servers

import (
	"context"
	"errors"
	"time"
)

// Server is one row in the future browser.
type Server struct {
	Game       string        `json:"game"`
	Address    string        `json:"address"`
	Name       string        `json:"name"`
	Map        string        `json:"map"`
	Mode       string        `json:"mode"`
	Players    int           `json:"players"`
	MaxPlayers int           `json:"maxPlayers"`
	Ping       time.Duration `json:"ping"`
}

// Source lists servers for one game.
type Source interface {
	Game() string
	List(ctx context.Context) ([]Server, error)
}

// Joiner joins a server in one game, through a method the game documents.
type Joiner interface {
	Game() string
	Join(ctx context.Context, s Server) error
}

// ErrUndocumented is returned until the game exposes a documented API.
var ErrUndocumented = errors.New("not available yet: waiting for the game's developers to document a server query and join method")

// Status is what the UI shows on the Servers page.
type Status struct {
	Available bool     `json:"available"`
	Reason    string   `json:"reason"`
	Needs     []string `json:"needs"`
}

// Sources returns registered sources (none yet).
func Sources() []Source { return nil }

// Joiners returns registered joiners (none yet).
func Joiners() []Joiner { return nil }

// Available reports the browser's state.
func Available() Status {
	return Status{
		Available: false,
		Reason:    ErrUndocumented.Error(),
		Needs: []string{
			"Project Reclaimer: a documented per-server info query (the discovery list has addresses only) and a join argument or URI",
			"Reach Renaissance: a public server list API and a join argument or URI",
		},
	}
}
