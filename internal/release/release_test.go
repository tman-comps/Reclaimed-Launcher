package release

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestParseSums(t *testing.T) {
	h := strings.Repeat("ab", 32)
	got, err := ParseSums(strings.NewReader("# comment\n" + h + "  a.exe\n" + strings.ToUpper(h) + " *dir/b c.zip\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got["a.exe"] != h || got["dir/b c.zip"] != h {
		t.Fatalf("got %v", got)
	}
	for _, bad := range []string{"", "nothex  a.exe\n", h + "\n", h[:60] + "  a\n"} {
		if _, err := ParseSums(strings.NewReader(bad)); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.0", "1.2", 0},
		{"1.10.0", "1.9.9", 1},
		{"0.1.2", "0.1.10", -1},
		{"1.0.0-beta", "1.0.0", -1},
		{"1.0.0-beta.2", "1.0.0-beta.10", 1}, // string compare on prerelease; documented limitation
		{"2.0", "1.99", 1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestPickPrefersNewNaming(t *testing.T) {
	r := &Release{Assets: []Asset{
		{Name: "project-reclaimer-dedicated-v1.4.0.exe"},
		{Name: "project-reclaimer-v1.4.0.exe"},
		{Name: "project-reclaimer-client-v1.4.0.exe"},
		{Name: "SHA256SUMS.txt"},
	}}
	pats := []Pattern{
		{Re: regexp.MustCompile(`^project-reclaimer-client-v?\d.*\.exe$`)},
		{Re: regexp.MustCompile(`^project-reclaimer-v?\d.*\.exe$`), Exclude: regexp.MustCompile(`dedicated`)},
	}
	if a := Pick(r, pats); a == nil || a.Name != "project-reclaimer-client-v1.4.0.exe" {
		t.Fatalf("picked %+v", a)
	}
}

func TestLatestETagAndRateLimit(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/repos/o/limited/releases/latest" {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("If-None-Match") == `"e1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"e1"`)
		fmt.Fprint(w, `{"tag_name":"v1.0.0","assets":[]}`)
	}))
	defer srv.Close()
	c := NewClient("test")
	c.BaseURL = srv.URL
	for i := 0; i < 2; i++ {
		r, err := c.Latest(context.Background(), "o/r")
		if err != nil || r.Tag != "v1.0.0" {
			t.Fatalf("try %d: %v %+v", i, err, r)
		}
	}
	if _, err := c.Latest(context.Background(), "o/limited"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("want rate limit error, got %v", err)
	}
}
