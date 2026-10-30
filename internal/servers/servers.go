// Package servers asks Project Reclaimer servers about themselves, for the
// launcher's server browser (Phase 3). Formats: docs/server-browser.md.
//
// The game's own browser does the same: for each address from the discovery
// masters it opens a QUIC connection (TLS server name "reclaimer", no ALPN,
// a self-signed certificate), opens one bidirectional stream, sends a 4-byte
// big-endian length followed by the JSON {"status":2}, and reads one reply
// framed the same way. Nothing else is sent: the launcher never starts a
// join itself (that sends the player's identity); Join hands the address to
// the game.
package servers

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/quic-go/quic-go"
)

// Server is one row in the browser.
type Server struct {
	Address          string   `json:"address"`
	Name             string   `json:"name"`
	HostName         string   `json:"hostName,omitempty"`
	Map              string   `json:"map"`
	Mode             string   `json:"mode"`
	Variant          string   `json:"variant,omitempty"`
	Players          int      `json:"players"`
	Capacity         int      `json:"capacity"`
	PasswordRequired bool     `json:"passwordRequired"`
	Version          string   `json:"version"`
	Mods             int      `json:"mods"`
	PlayerNames      []string `json:"playerNames,omitempty"`
	PingMs           int      `json:"pingMs"`
}

// status is the server's reply as sent (2026-10-30, game v0.9.23).
type status struct {
	Name             string            `json:"name"`
	HostName         string            `json:"host_name"`
	Map              string            `json:"map"`
	Mode             string            `json:"mode"`
	Variant          string            `json:"variant"`
	Players          int               `json:"players"`
	Capacity         int               `json:"capacity"`
	PasswordRequired bool              `json:"password_required"`
	Version          string            `json:"version"`
	Mods             []json.RawMessage `json:"mods"`
	PlayerNames      []string          `json:"player_names"`
}

// Request is the status query the game's browser sends.
var Request = []byte(`{"status":2}`)

// MaxReply caps a status reply (about 2 KB for a full 120-player server).
const MaxReply = 256 << 10

// ServerName is the TLS server name every Reclaimer server answers to.
const ServerName = "reclaimer"

var addrRe = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}:(\d{1,5})$`)

// ValidAddress reports whether s is host:port as the game's join links take
// it (the same rule as projectreclaimer.dev/join).
func ValidAddress(s string) bool {
	m := addrRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	var port int
	fmt.Sscan(m[1], &port)
	return port >= 1 && port <= 65535
}

// JoinLink is the link the game registers for joining a server.
func JoinLink(addr string) string { return "reclaimer://join/" + addr }

func tlsConfig() *tls.Config {
	// Servers use throwaway self-signed certificates; the game identifies
	// them by their signed discovery records, not by certificate. Only
	// public status is read here, so the certificate isn't checked.
	return &tls.Config{ServerName: ServerName, InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}
}

// writeFrame and readFrame are the 4-byte length + JSON framing.
func writeFrame(w io.Writer, body []byte) error {
	_, err := w.Write(append(binary.BigEndian.AppendUint32(nil, uint32(len(body))), body...))
	return err
}

func readFrame(r io.Reader, max uint32) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(n[:])
	if size > max {
		return nil, fmt.Errorf("reply too large (%d bytes)", size)
	}
	b := make([]byte, size)
	_, err := io.ReadFull(r, b)
	return b, err
}

// clean trims and drops control characters from a server-supplied string.
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > max {
		s = strings.ToValidUTF8(s[:max], "")
	}
	return s
}

func parseStatus(addr string, body []byte) (Server, error) {
	var st status
	if err := json.Unmarshal(body, &st); err != nil {
		return Server{}, fmt.Errorf("the server described itself in a way the launcher can't read: %w", err)
	}
	s := Server{
		Address: addr, Name: clean(st.Name, 96), HostName: clean(st.HostName, 96), Map: clean(st.Map, 96),
		Mode: clean(st.Mode, 64), Variant: clean(st.Variant, 96), Players: st.Players, Capacity: st.Capacity,
		PasswordRequired: st.PasswordRequired, Version: clean(st.Version, 32), Mods: len(st.Mods),
	}
	for _, n := range st.PlayerNames {
		if len(s.PlayerNames) == 256 {
			break
		}
		s.PlayerNames = append(s.PlayerNames, clean(n, 64))
	}
	if s.Name == "" {
		s.Name = addr
	}
	return s, nil
}

// Querier asks servers for their status over one UDP socket.
type Querier struct {
	tr *quic.Transport
}

// NewQuerier opens the UDP socket queries use.
func NewQuerier() (*Querier, error) {
	c, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	return &Querier{tr: &quic.Transport{Conn: c}}, nil
}

// Close releases the socket.
func (q *Querier) Close() error { return q.tr.Close() }

// Query asks one server for its status.
func (q *Querier) Query(ctx context.Context, addr string) (Server, error) {
	if !ValidAddress(addr) {
		return Server{}, fmt.Errorf("%q isn't a host:port address", addr)
	}
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return Server{}, err
	}
	start := time.Now()
	conn, err := q.tr.Dial(ctx, ua, tlsConfig(), &quic.Config{HandshakeIdleTimeout: 3 * time.Second, MaxIdleTimeout: 5 * time.Second})
	if err != nil {
		return Server{}, err
	}
	ping := time.Since(start)
	defer conn.CloseWithError(0, "")
	st, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return Server{}, err
	}
	if dl, ok := ctx.Deadline(); ok {
		st.SetDeadline(dl)
	}
	if err := writeFrame(st, Request); err != nil {
		return Server{}, err
	}
	st.Close() // done sending; the server replies and closes its side
	body, err := readFrame(st, MaxReply)
	if err != nil {
		return Server{}, err
	}
	s, err := parseStatus(addr, body)
	s.PingMs = int(ping.Milliseconds())
	return s, err
}

// Browse queries every address, at most parallel at once, each within
// timeout, and calls each (one call at a time) as answers arrive.
func (q *Querier) Browse(ctx context.Context, addrs []string, parallel int, timeout time.Duration, each func(Server, error)) {
	var mu sync.Mutex
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, a := range addrs {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			defer func() { <-sem }()
			qctx, cancel := context.WithTimeout(ctx, timeout)
			s, err := q.Query(qctx, a)
			cancel()
			if err != nil {
				s = Server{Address: a}
			}
			mu.Lock()
			each(s, err)
			mu.Unlock()
		}(a)
	}
	wg.Wait()
}

// Sort orders servers the way the browser shows them: most players first,
// then lowest ping, then name.
func Sort(list []Server) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Players != b.Players {
			return a.Players > b.Players
		}
		if a.PingMs != b.PingMs {
			return a.PingMs < b.PingMs
		}
		return a.Name < b.Name
	})
}

// ErrNoAddresses is returned when discovery gave nothing to ask.
var ErrNoAddresses = errors.New("the discovery masters listed no servers")
