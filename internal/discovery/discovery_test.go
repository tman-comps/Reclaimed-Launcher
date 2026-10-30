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

func TestBothDown(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"nope":1}`) }))
	defer bad.Close()
	r := Fetch(context.Background(), http.DefaultClient, []string{bad.URL})
	if r.OK || r.Error == "" {
		t.Fatalf("got %+v", r)
	}
}
