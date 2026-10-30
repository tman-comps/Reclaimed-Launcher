package art

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestURLAndHandler(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "reclaimer-wide.png"), []byte("\x89PNG\r\n\x1a\nxx"), 0o644)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("no"), 0o644)
	if u := URL(dir, "reclaimer", Wide); !strings.HasPrefix(u, "/art/reclaimer-wide.png?v=") {
		t.Fatalf("url = %q", u)
	}
	if u := URL(dir, "reclaimer", Tall); u != "/art/default/reclaimer-tall.webp" {
		t.Fatalf("tall should fall back to built-in, got %q", u)
	}
	if Source(dir, "reclaimer", Wide) != "custom" || Source(dir, "reach-renaissance", Wide) != "built-in" || Source(dir, "nope", Wide) != "" {
		t.Fatal("wrong sources")
	}
	h := Handler(dir)
	for path, want := range map[string]int{
		"/art/reclaimer-wide.png":                 200,
		"/art/secret.txt":                         404,
		"/art/../art/secret.txt":                  404,
		"/art/reclaimer-tall.png":                 404,
		"/art/default/reclaimer-tall.webp":        200,
		"/art/default/reach-renaissance-wide.png": 200,
		"/art/default/../art.go":                  404,
		"/art/default/missing-tall.png":           404,
		"/other/reclaimer-wide.png":               404,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}
}
