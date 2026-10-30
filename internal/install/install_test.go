package install

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func TestDownloadVerifies(t *testing.T) {
	payload := []byte("hello game")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	defer srv.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "x.exe")

	if err := Download(context.Background(), srv.Client(), "t", srv.URL, dest, sha(payload), 1<<20, nil); err != nil {
		t.Fatal(err)
	}
	err := Download(context.Background(), srv.Client(), "t", srv.URL, dest, sha([]byte("other")), 1<<20, nil)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("want checksum error, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("bad download should be deleted")
	}
	if err := Download(context.Background(), srv.Client(), "t", srv.URL, dest, "", 1<<20, nil); err == nil {
		t.Fatal("must refuse without a checksum")
	}
	if err := Download(context.Background(), srv.Client(), "t", srv.URL, dest, sha(payload), 4, nil); err == nil {
		t.Fatal("must enforce size limit")
	}
}

func makeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	f.Close()
}

func TestUnzipStripsTopFolder(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "a.zip")
	makeZip(t, z, map[string]string{"ReachRenaissance/Reach Renaissance.exe": "x", "ReachRenaissance/data/a.bin": "y"})
	out := filepath.Join(dir, "out")
	if err := Unzip(z, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "Reach Renaissance.exe")); err != nil {
		t.Fatal(err)
	}
}

func TestUnzipRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "a.zip")
	makeZip(t, z, map[string]string{"ok.txt": "x", "../evil.txt": "y"})
	if err := Unzip(z, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("file escaped destination")
	}
}

func TestSwapDirKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	live, staged, prev := filepath.Join(dir, "live"), filepath.Join(dir, "staged"), filepath.Join(dir, "prev")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "v"), []byte("1"), 0o644)
	os.MkdirAll(staged, 0o755)
	os.WriteFile(filepath.Join(staged, "v"), []byte("2"), 0o644)
	if err := SwapDir(staged, live, prev); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(live, "v")); string(b) != "2" {
		t.Fatal("live not updated")
	}
	if b, _ := os.ReadFile(filepath.Join(prev, "v")); string(b) != "1" {
		t.Fatal("previous not kept")
	}
}
