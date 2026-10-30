package servers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// A reply shaped like the one a real server sent on 2026-10-30.
const realReply = `{"name":"DaBois | Hugegrass 120 Player (Updated)","host_name":"Juices Server",` +
	`"map":"Hugegrass · Custom","mode":"Capture the Flag","players":99,"capacity":120,"ready":true,` +
	`"password_required":false,"mods":[{"id":"3a88","title":"Hugegrass","maps":[],"size":2118599548,"shared":true}],` +
	`"version":"0.9.23","variant":"62Player CTF","map_variant":"Hugegrass HugeTeamBattle",` +
	`"player_names":["its_Grippy","HangoverJoe"],"blocks_vpn":true}`

// fakeServer answers like a Reclaimer server: one framed request, one
// framed reply. It records what it was sent.
func fakeServer(t *testing.T, reply func(req []byte) []byte) (addr string, got chan []byte) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "rcgen self signed cert"},
		DNSNames: []string{"reclaimer"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan []byte, 8)
	go func() {
		for {
			c, err := ln.Accept(context.Background())
			if err != nil {
				return
			}
			go func() {
				if c.ConnectionState().TLS.ServerName != "reclaimer" {
					c.CloseWithError(1, "wrong server name")
					return
				}
				s, err := c.AcceptStream(context.Background())
				if err != nil {
					return
				}
				req, err := readFrame(s, 1024)
				if err != nil {
					return
				}
				got <- req
				s.Write(reply(req))
				s.Close()
				<-c.Context().Done()
			}()
		}
	}()
	return ln.Addr().String(), got
}

func framed(body string) []byte {
	return append([]byte{0, 0, byte(len(body) >> 8), byte(len(body))}, body...)
}

func TestQuery(t *testing.T) {
	addr, got := fakeServer(t, func([]byte) []byte { return framed(realReply) })
	q, err := NewQuerier()
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := q.Query(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	if req := <-got; string(req) != `{"status":2}` {
		t.Errorf("request = %q", req)
	}
	if s.Name != "DaBois | Hugegrass 120 Player (Updated)" || s.Map != "Hugegrass · Custom" || s.Mode != "Capture the Flag" ||
		s.Players != 99 || s.Capacity != 120 || s.PasswordRequired || s.Version != "0.9.23" || s.Mods != 1 ||
		len(s.PlayerNames) != 2 || s.Address != addr {
		t.Errorf("server = %+v", s)
	}
}

func TestQueryRejectsOversizeReply(t *testing.T) {
	addr, _ := fakeServer(t, func([]byte) []byte { return []byte{0x7f, 0xff, 0xff, 0xff} })
	q, _ := NewQuerier()
	defer q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := q.Query(ctx, addr); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v", err)
	}
}

func TestQueryUnreadableReply(t *testing.T) {
	addr, _ := fakeServer(t, func([]byte) []byte { return framed("not json") })
	q, _ := NewQuerier()
	defer q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := q.Query(ctx, addr); err == nil {
		t.Fatal("expected an error")
	}
}

func TestBrowseKeepsGoingPastDeadServers(t *testing.T) {
	good, _ := fakeServer(t, func([]byte) []byte { return framed(realReply) })
	q, _ := NewQuerier()
	defer q.Close()
	// 127.0.0.1:9 has nothing listening: it must time out, not stop the rest.
	var ok, failed int
	q.Browse(context.Background(), []string{"127.0.0.1:9", good, "bad address"}, 2, 2*time.Second, func(s Server, err error) {
		if err != nil {
			failed++
		} else if s.Players == 99 {
			ok++
		}
	})
	if ok != 1 || failed != 2 {
		t.Fatalf("ok=%d failed=%d", ok, failed)
	}
}

func TestValidAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"1.2.3.4:49177": true, "play.halo.latorre.cloud:49178": true, "h3.jlab.systems:1": true,
		"1.2.3.4": false, "1.2.3.4:0": false, "1.2.3.4:70000": false, "evil.com:80/x": false,
		"a b:1": false, "reclaimer://join/1.2.3.4:1": false, "": false,
	} {
		if ValidAddress(addr) != want {
			t.Errorf("ValidAddress(%q) = %v", addr, !want)
		}
	}
	if JoinLink("1.2.3.4:49177") != "reclaimer://join/1.2.3.4:49177" {
		t.Error("join link format")
	}
}

func TestCleanAndSort(t *testing.T) {
	s, err := parseStatus("1.2.3.4:1", framed(`{"name":"  Bad\u0007Name\n","players":3}`)[4:])
	if err != nil || s.Name != "BadName" {
		t.Fatalf("name = %q, %v", s.Name, err)
	}
	list := []Server{{Name: "b", Players: 2, PingMs: 50}, {Name: "a", Players: 9, PingMs: 200}, {Name: "c", Players: 2, PingMs: 20}}
	Sort(list)
	if list[0].Name != "a" || list[1].Name != "c" || list[2].Name != "b" {
		t.Errorf("order = %v", list)
	}
}
