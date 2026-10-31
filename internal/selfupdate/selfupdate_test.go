package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reclaimed/internal/release"
)

func fakeGitHub(t *testing.T, tag string, exe []byte, sums string) *release.Client {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.invalid","body":"notes","assets":[
				{"name":"Reclaimed.exe","size":%d,"browser_download_url":"%s/exe"},
				{"name":"SHA256SUMS.txt","size":1,"browser_download_url":"%s/sums"}]}`, tag, len(exe), srv.URL, srv.URL)
		case "/exe":
			w.Write(exe)
		case "/sums":
			fmt.Fprint(w, sums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	gh := release.NewClient("test")
	gh.BaseURL = srv.URL
	return gh
}

func TestCheck(t *testing.T) {
	gh := fakeGitHub(t, "v0.5.0", []byte("x"), "")
	for cur, want := range map[string]bool{"0.4.1": true, "0.5.0": false, "0.6.0": false} {
		info, err := Check(context.Background(), gh, cur)
		if err != nil || info.Available != want || info.Latest != "0.5.0" {
			t.Errorf("%s: %+v %v", cur, info, err)
		}
	}
}

func TestApply(t *testing.T) {
	newExe := []byte("new launcher")
	h := sha256.Sum256(newExe)
	dir := t.TempDir()
	exe := filepath.Join(dir, "Reclaimed.exe")
	os.WriteFile(exe, []byte("old launcher"), 0o755)

	// A file that doesn't match the sums changes nothing.
	gh := fakeGitHub(t, "v0.5.0", newExe, fmt.Sprintf("%064x  Reclaimed.exe\n", 0))
	if err := Apply(context.Background(), gh, http.DefaultClient, "test", exe, filepath.Join(dir, "staging"), nil); err == nil {
		t.Fatal("want a checksum error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old launcher" {
		t.Fatal("exe changed after a failed update")
	}

	gh = fakeGitHub(t, "v0.5.0", newExe, hex.EncodeToString(h[:])+"  Reclaimed.exe\n")
	if err := Apply(context.Background(), gh, http.DefaultClient, "test", exe, filepath.Join(dir, "staging"), nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new launcher" {
		t.Fatal("exe not replaced")
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old launcher" {
		t.Fatal("old exe not kept")
	}
	Cleanup(exe)
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Fatal("old exe not cleaned up")
	}
}
