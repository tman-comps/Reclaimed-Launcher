package servers

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"reclaimed/internal/discovery"
)

// TestLiveBrowse asks the real servers, like the launcher's Refresh. It runs
// only with RECLAIMED_LIVE=1 (it needs the internet):
//
//	$env:RECLAIMED_LIVE=1; go test -run Live -v ./internal/servers/
func TestLiveBrowse(t *testing.T) {
	if os.Getenv("RECLAIMED_LIVE") == "" {
		t.Skip("set RECLAIMED_LIVE=1 to query the real servers")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r := discovery.Fetch(ctx, &http.Client{Timeout: 10 * time.Second}, discovery.DefaultEndpoints)
	addrs := r.Addresses()
	if len(addrs) == 0 {
		t.Fatal("no addresses:", r.Error)
	}
	q, err := NewQuerier()
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	start := time.Now()
	var list []Server
	failures := map[string]int{}
	q.Browse(ctx, addrs, 48, 3*time.Second, func(s Server, err error) {
		if err != nil {
			msg := err.Error()
			if len(msg) > 40 {
				msg = msg[:40]
			}
			failures[msg]++
			return
		}
		list = append(list, s)
	})
	Sort(list)
	players := 0
	for _, s := range list {
		players += s.Players
	}
	t.Logf("%d of %d answered in %v; %d players", len(list), len(addrs), time.Since(start).Round(time.Millisecond), players)
	for i := 0; i < 8 && i < len(list); i++ {
		s := list[i]
		t.Logf("  %3d/%-3d %4dms  %s | %s | %s", s.Players, s.Capacity, s.PingMs, s.Name, s.Map, s.Mode)
	}
	for msg, n := range failures {
		t.Logf("  failed x%d: %s", n, msg)
	}
	if len(list) < len(addrs)/2 {
		t.Errorf("fewer than half answered")
	}
}
