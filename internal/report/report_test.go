package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSniff(t *testing.T) {
	cases := map[string]string{
		`{"fov": 90}`:                 "JSON",
		"[Video]\nWidth=1920\n":       "INI",
		"bind w +forward\nfov 90\n":   "console-style cfg (command value)",
		"\x00\x01\x02":                "binary",
		"<?xml version=\"1.0\"?><a/>": "XML",
		"\r\n  \n":                    "empty", // used to panic on t[0]
	}
	for in, want := range cases {
		name := "x.cfg"
		if got := Sniff(name, []byte(in), int64(len(in))); got != want {
			t.Errorf("Sniff(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildMasksAndSkipsLogs(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"video":{"fov":95,"width":2560},"auth_token":"abc","lastServer":"10.1.2.3:1234"}`), 0o644)
	os.MkdirAll(filepath.Join(src, "runs", "1"), 0o755)
	os.WriteFile(filepath.Join(src, "runs", "1", "log.txt"), []byte("secret log"), 0o644)
	out, err := Build(t.TempDir(), []Target{{"Test", src}, {"Missing", filepath.Join(src, "nope")}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	s := string(b)
	for _, want := range []string{"* video.fov = 95", "auth_token = <masked>", "x.x.x.x:1234", "runs/  (skipped: 1", "folder not found"} {
		if !strings.Contains(s, want) {
			t.Errorf("report missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "secret log") || strings.Contains(s, "10.1.2.3") {
		t.Error("report leaked data")
	}
}
