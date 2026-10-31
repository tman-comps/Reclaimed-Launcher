// Package reachlist counts Reach Renaissance's public servers and players
// from its master server's list (docs/server-browser.md). It only reads the
// list; joining stays in the game.
package reachlist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Master is Reach Renaissance's default master server (from its rr-browser).
const Master = "https://2-29-30-1.sslip.io:11700"

// Count is the size of the public list.
type Count struct {
	OK        bool      `json:"ok"`
	Servers   int       `json:"servers"`
	Players   int       `json:"players"`
	Error     string    `json:"error,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
}

type listing struct {
	Servers []struct {
		Signed struct {
			Listing struct {
				Players int `json:"players"`
			} `json:"listing"`
		} `json:"signed"`
	} `json:"servers"`
}

// Fetch reads the master's server list and counts it.
func Fetch(ctx context.Context, client *http.Client, ua string) Count {
	c := Count{FetchedAt: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Master+"/v1/servers", nil)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.Error = "server list: " + resp.Status
		return c
	}
	c, err = parse(io.LimitReader(resp.Body, 8<<20))
	c.FetchedAt = time.Now()
	if err != nil {
		c.Error = err.Error()
	}
	return c
}

func parse(r io.Reader) (Count, error) {
	var l listing
	if err := json.NewDecoder(r).Decode(&l); err != nil {
		return Count{}, fmt.Errorf("server list: %w", err)
	}
	c := Count{OK: true, Servers: len(l.Servers)}
	for _, s := range l.Servers {
		if p := s.Signed.Listing.Players; p > 0 {
			c.Players += p
		}
	}
	return c, nil
}
