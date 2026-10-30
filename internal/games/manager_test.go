package games

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reclaimed/internal/paths"
	"reclaimed/internal/release"
	"reclaimed/internal/state"
	"reclaimed/internal/steam"
)

func hexsum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// fakeGitHub serves one release per repo plus its files.
type fakeGitHub struct {
	srv    *httptest.Server
	files  map[string][]byte   // asset name -> content
	tags   map[string]string   // repo -> tag
	repos  map[string][]string // repo -> asset names
	badSum bool
}

func newFake(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{files: map[string][]byte{}, tags: map[string]string{}, repos: map[string][]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) publish(repo, tag string, files map[string][]byte) {
	f.tags[repo] = tag
	f.repos[repo] = nil
	var sums strings.Builder
	for n, b := range files {
		f.files[n] = b
		f.repos[repo] = append(f.repos[repo], n)
		sum := hexsum(b)
		if f.badSum {
			sum = hexsum([]byte("tampered"))
		}
		fmt.Fprintf(&sums, "%s  %s\n", sum, n)
	}
	f.files[repo+"/SHA256SUMS.txt"] = []byte(sums.String())
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/repos/") && strings.HasSuffix(r.URL.Path, "/releases/latest") {
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases/latest")
		tag, ok := f.tags[repo]
		if !ok {
			http.NotFound(w, r)
			return
		}
		var assets []string
		for _, n := range f.repos[repo] {
			assets = append(assets, fmt.Sprintf(`{"name":%q,"size":%d,"browser_download_url":"%s/dl/%s"}`, n, len(f.files[n]), f.srv.URL, n))
		}
		assets = append(assets, fmt.Sprintf(`{"name":"SHA256SUMS.txt","size":1,"browser_download_url":"%s/dl/%s/SHA256SUMS.txt"}`, f.srv.URL, repo))
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"x","assets":[%s]}`, tag, strings.Join(assets, ","))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/dl/") {
		if b, ok := f.files[strings.TrimPrefix(r.URL.Path, "/dl/")]; ok {
			w.Write(b)
			return
		}
	}
	http.NotFound(w, r)
}

func setup(t *testing.T, f *fakeGitHub, mcc steam.MCCInfo) *Manager {
	root := t.TempDir()
	l := paths.New(root)
	st, err := state.Open(filepath.Join(l.Data, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	gh := release.NewClient("test")
	gh.BaseURL = f.srv.URL
	return NewManager(l, st, gh, "test", func() steam.MCCInfo { return mcc })
}

func okMCC() steam.MCCInfo {
	return steam.MCCInfo{Found: true, Path: filepath.Join(os.TempDir(), "nowhere-mcc"), Components: []steam.Component{
		{ID: "halo3", Installed: true}, {ID: "haloreach", Installed: true},
	}}
}

func noop(string, int64, int64) {}

func TestReclaimerInstallUpdateAndInGameUpdate(t *testing.T) {
	f := newFake(t)
	repo := ByID("reclaimer").Repo
	f.publish(repo, "v1.0.0", map[string][]byte{
		"project-reclaimer-client-v1.0.0.exe":    []byte("client 1.0"),
		"project-reclaimer-dedicated-v1.0.0.exe": []byte("server"),
	})
	m := setup(t, f, okMCC())
	ctx := context.Background()
	m.CheckUpdates(ctx)
	s := m.Status("reclaimer", nil)
	if s.Installed || s.Latest != "1.0.0" || !s.RequirementOK {
		t.Fatalf("before install: %+v", s)
	}
	if err := m.Install(ctx, "reclaimer", noop); err != nil {
		t.Fatal(err)
	}
	s = m.Status("reclaimer", nil)
	if !s.Installed || s.Version != "1.0.0" || s.VersionSource != "launcher" || s.UpdateAvailable {
		t.Fatalf("after install: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(m.L.GameDir("reclaimer"), "project-reclaimer-dedicated-v1.0.0.exe")); err == nil {
		t.Fatal("dedicated server must not be installed")
	}

	// New release: launcher offers an update.
	f.publish(repo, "v1.1.0", map[string][]byte{"project-reclaimer-client-v1.1.0.exe": []byte("client 1.1")})
	m.CheckUpdates(ctx)
	if s = m.Status("reclaimer", nil); !s.UpdateAvailable || s.Latest != "1.1.0" {
		t.Fatalf("expected update: %+v", s)
	}

	// The game's own updater replaces the exe in place (same filename).
	exe := filepath.Join(m.L.GameDir("reclaimer"), "project-reclaimer-client-v1.0.0.exe")
	os.WriteFile(exe, []byte("client 1.1"), 0o644)
	s = m.Status("reclaimer", nil)
	if s.Version != "1.1.0" || s.VersionSource != "release" || s.UpdateAvailable {
		t.Fatalf("in-game update not recognised: %+v", s)
	}

	// Updating through the launcher moves the old exe aside.
	f.publish(repo, "v1.2.0", map[string][]byte{"project-reclaimer-client-v1.2.0.exe": []byte("client 1.2")})
	if err := m.Install(ctx, "reclaimer", noop); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(m.L.GameDir("reclaimer"))
	if len(ents) != 1 || ents[0].Name() != "project-reclaimer-client-v1.2.0.exe" {
		t.Fatalf("game folder: %v", ents)
	}
	if _, err := os.Stat(filepath.Join(m.L.Previous, "reclaimer", "project-reclaimer-client-v1.0.0.exe")); err != nil {
		t.Fatal("previous exe not kept")
	}
}

func TestUnknownBuildAfterSelfUpdateIsNotNagged(t *testing.T) {
	f := newFake(t)
	repo := ByID("reclaimer").Repo
	f.publish(repo, "v1.0.0", map[string][]byte{"project-reclaimer-client-v1.0.0.exe": []byte("a")})
	m := setup(t, f, okMCC())
	ctx := context.Background()
	if err := m.Install(ctx, "reclaimer", noop); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(m.L.GameDir("reclaimer"), "project-reclaimer-client-v1.0.0.exe"), []byte("hotfix"), 0o644)
	m.CheckUpdates(ctx)
	s := m.Status("reclaimer", nil)
	if !s.UpdatedInGame || s.UpdateAvailable {
		t.Fatalf("got %+v", s)
	}
}

func TestChecksumMismatchInstallsNothing(t *testing.T) {
	f := newFake(t)
	f.badSum = true
	f.publish(ByID("reclaimer").Repo, "v1.0.0", map[string][]byte{"project-reclaimer-client-v1.0.0.exe": []byte("a")})
	m := setup(t, f, okMCC())
	if err := m.Install(context.Background(), "reclaimer", noop); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want checksum error, got %v", err)
	}
	if _, err := os.Stat(m.L.GameDir("reclaimer")); err == nil {
		ents, _ := os.ReadDir(m.L.GameDir("reclaimer"))
		if len(ents) > 0 {
			t.Fatal("files installed despite mismatch")
		}
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return buf.Bytes()
}

func TestReachZipInstallAndSwap(t *testing.T) {
	f := newFake(t)
	repo := ByID("reach-renaissance").Repo
	f.publish(repo, "v0.1.2", map[string][]byte{"ReachRenaissance-v0.1.2.zip": zipBytes(t, map[string]string{
		"ReachRenaissance/Reach Renaissance.exe": "rr 0.1.2", "ReachRenaissance/bin/x.dll": "d",
	})})
	m := setup(t, f, okMCC())
	ctx := context.Background()
	if err := m.Install(ctx, "reach-renaissance", noop); err != nil {
		t.Fatal(err)
	}
	m.CheckUpdates(ctx)
	s := m.Status("reach-renaissance", nil)
	if !s.Installed || s.Version != "0.1.2" || s.UpdateAvailable {
		t.Fatalf("after install: %+v", s)
	}
	f.publish(repo, "v0.1.3", map[string][]byte{"ReachRenaissance-v0.1.3.zip": zipBytes(t, map[string]string{
		"Reach Renaissance.exe": "rr 0.1.3",
	})})
	m.CheckUpdates(ctx)
	if s = m.Status("reach-renaissance", nil); !s.UpdateAvailable {
		t.Fatalf("want update: %+v", s)
	}
	if err := m.Install(ctx, "reach-renaissance", noop); err != nil {
		t.Fatal(err)
	}
	if s = m.Status("reach-renaissance", nil); s.Version != "0.1.3" {
		t.Fatalf("after update: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(m.L.Previous, "reach-renaissance", "bin", "x.dll")); err != nil {
		t.Fatal("previous version not kept")
	}
	if ents, _ := os.ReadDir(m.L.Staging); len(ents) != 0 {
		t.Fatalf("staging not cleaned: %v", ents)
	}
}

func TestReachRefusedInsideMCC(t *testing.T) {
	f := newFake(t)
	f.publish(ByID("reach-renaissance").Repo, "v0.1.2", map[string][]byte{"ReachRenaissance-v0.1.2.zip": zipBytes(t, map[string]string{"Reach Renaissance.exe": "x"})})
	m := setup(t, f, okMCC())
	mcc := okMCC()
	mcc.Path = m.L.Root // launcher placed inside MCC
	m.MCC = func() steam.MCCInfo { return mcc }
	if err := m.Install(context.Background(), "reach-renaissance", noop); err == nil {
		t.Fatal("expected refusal")
	}
	if s := m.Status("reach-renaissance", nil); s.Blocker == "" {
		t.Fatal("expected blocker in status")
	}
}

func TestRequirementMissing(t *testing.T) {
	f := newFake(t)
	mcc := okMCC()
	mcc.Components[1].Installed = false
	m := setup(t, f, mcc)
	if s := m.Status("reach-renaissance", nil); s.RequirementOK || !strings.Contains(s.RequirementMsg, "Reach") {
		t.Fatalf("got %+v", s)
	}
}

func TestLinkedInstallIsNeverWritten(t *testing.T) {
	f := newFake(t)
	f.publish(ByID("reclaimer").Repo, "v1.0.0", map[string][]byte{"project-reclaimer-client-v1.0.0.exe": []byte("a")})
	m := setup(t, f, okMCC())
	ext := t.TempDir()
	os.WriteFile(filepath.Join(ext, "project-reclaimer.exe"), []byte("old"), 0o644)
	if err := m.Link("reclaimer", ext); err != nil {
		t.Fatal(err)
	}
	m.CheckUpdates(context.Background())
	s := m.Status("reclaimer", nil)
	if !s.Installed || !s.Linked || s.UpdateAvailable {
		t.Fatalf("got %+v", s)
	}
	if err := m.Install(context.Background(), "reclaimer", noop); err == nil {
		t.Fatal("install into linked folder must be refused")
	}
}
