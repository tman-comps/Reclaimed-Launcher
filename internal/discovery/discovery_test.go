package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFallbackToBackup(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"records":[{"a":1},{"a":2},{"a":3}],"masters":["m1","m2"]}`)
	}))
	defer good.Close()
	r := Fetch(context.Background(), http.DefaultClient, []string{bad.URL, good.URL})
	if !r.OK || r.Servers != 3 || r.Masters != 2 || r.Endpoint != good.URL {
		t.Fatalf("got %+v", r)
	}
}

// The record shape seen on the live masters (2026-10-30).
func TestAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"records":[
			{"record":{"key":"k1","address":"play.halo.latorre.cloud:49178","issued":1,"expires":181},"signature":"s"},
			{"record":{"key":"k2","origin":"http://1.2.3.4:49175","address":"1.2.3.4:49177","issued":1,"expires":181},"signature":"s"},
			{"record":{"key":"k3","address":"1.2.3.4:49177"},"signature":"s"},
			{"record":{"key":"k4"}},
			"junk"],"masters":[]}`)
	}))
	defer srv.Close()
	got := Fetch(context.Background(), http.DefaultClient, []string{srv.URL}).Addresses()
	if fmt.Sprint(got) != "[play.halo.latorre.cloud:49178 1.2.3.4:49177]" {
		t.Fatalf("addresses = %v", got)
	}
}

func TestBothDown(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"nope":1}`) }))
	defer bad.Close()
	r := Fetch(context.Background(), http.DefaultClient, []string{bad.URL})
	if r.OK || r.Error == "" {
		t.Fatalf("got %+v", r)
	}
}
