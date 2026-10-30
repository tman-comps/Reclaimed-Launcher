// Package report builds a read-only inventory of each game's user-data
// folder, to decide how (and whether) Phase 2 settings pages can be built.
// It never writes outside the reports folder it is given.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Target is one folder to inspect.
type Target struct {
	Game string
	Dir  string
}

const (
	maxDepth     = 4
	maxFiles     = 400
	maxParseSize = 512 << 10
	maxKeys      = 300
)

// skipDirs hold logs or crash data, not settings. They are counted, not listed.
var skipDirs = map[string]bool{"runs": true, "logs": true, "crashes": true, "crashdumps": true, "cache": true, "screenshots": true}

var settingHint = regexp.MustCompile(`(?i)(fov|field.?of.?view|sens|mouse|resolution|width|height|fullscreen|window|vsync|fps|frame|refresh|volume|audio|sound|music|bind|keyboard|hotkey|input|controller|gamepad|bright|gamma|quality)`)

// skipDir reports folders that hold logs, caches or an embedded browser's
// profile (Reach Renaissance keeps one in bin\*.WebView2), not settings.
func skipDir(name string) bool {
	n := strings.ToLower(name)
	return skipDirs[n] || strings.HasSuffix(n, ".webview2") || n == "ebwebview"
}

// docFile reports files that are the game's documentation, not settings.
var docFile = regexp.MustCompile(`(?i)^(readme|license|licence|changelog|changes)([-_.].*)?\.(txt|md)$`)
var secretKey = regexp.MustCompile(`(?i)(token|secret|password|passwd|auth|session|cookie|private|signature|xuid|steamid|email)`)
var ipLike = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

// Build writes a report to dir and returns its path.
func Build(dir string, targets []Target) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Reclaimed Launcher settings report\nGenerated %s\n", time.Now().Format(time.RFC1123))
	b.WriteString("Read-only scan. Long strings, addresses and secret-looking values are masked.\n")
	b.WriteString("Lines marked * look like keybind/video/audio settings.\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "\n=== %s ===\nFolder: %s\n", t.Game, t.Dir)
		scan(&b, t.Dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, "settings-report-"+time.Now().Format("20060102-150405")+".txt")
	return out, os.WriteFile(out, []byte(b.String()), 0o644)
}

func scan(b *strings.Builder, root string) {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		b.WriteString("  (folder not found: has the game been run on this PC?)\n")
		return
	}
	n := 0
	fs.WalkDir(os.DirFS(root), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(b, "  %s: unreadable (%v)\n", p, err)
			return nil
		}
		if p == "." {
			return nil
		}
		depth := strings.Count(p, "/") + 1
		if d.IsDir() {
			if skipDir(d.Name()) {
				cnt := 0
				fs.WalkDir(os.DirFS(filepath.Join(root, p)), ".", func(_ string, e fs.DirEntry, _ error) error {
					if e != nil && !e.IsDir() {
						cnt++
					}
					return nil
				})
				fmt.Fprintf(b, "  %s/  (skipped: %d log/cache files)\n", p, cnt)
				return fs.SkipDir
			}
			if depth > maxDepth {
				fmt.Fprintf(b, "  %s/  (not scanned: too deep)\n", p)
				return fs.SkipDir
			}
			return nil
		}
		if n++; n > maxFiles {
			return fs.SkipAll
		}
		info, _ := d.Info()
		size, mod := int64(0), time.Time{}
		if info != nil {
			size, mod = info.Size(), info.ModTime()
		}
		if docFile.MatchString(d.Name()) {
			fmt.Fprintf(b, "\n  %s  [documentation, not read, %d bytes]\n", p, size)
			return nil
		}
		full := filepath.Join(root, filepath.FromSlash(p))
		var data []byte
		if size <= maxParseSize {
			data, _ = os.ReadFile(full)
		}
		format := Sniff(p, data, size)
		fmt.Fprintf(b, "\n  %s  [%s, %d bytes, modified %s]\n", p, format, size, mod.Format("2006-01-02 15:04"))
		if data != nil {
			describe(b, format, data)
		}
		return nil
	})
	if n > maxFiles {
		fmt.Fprintf(b, "  ... stopped after %d files\n", maxFiles)
	}
}

// Sniff guesses a file's format from its content (extension is a tiebreaker).
func Sniff(name string, data []byte, size int64) string {
	if data == nil {
		return "large file, not read"
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if len(data) == 0 {
		return "empty"
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		if len(data) >= 2 && (data[0] == 0xff && data[1] == 0xfe || data[0] == 0xfe && data[1] == 0xff) {
			return "UTF-16 text"
		}
		return "binary"
	}
	t := bytes.TrimSpace(data)
	if len(t) == 0 {
		return "empty"
	}
	if (t[0] == '{' || t[0] == '[') && json.Valid(t) {
		return "JSON"
	}
	if t[0] == '<' {
		return "XML"
	}
	ext := strings.ToLower(filepath.Ext(name))
	lines := strings.Split(string(t), "\n")
	eq, sp, sect := 0, 0, 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		switch {
		case l == "" || strings.HasPrefix(l, ";") || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "//"):
		case strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]"):
			sect++
		case strings.Contains(l, "="):
			eq++
		case strings.Contains(l, " "):
			sp++
		}
	}
	switch {
	case eq > 0 && (sect > 0 || ext == ".ini"):
		return "INI"
	case eq > 0:
		return "key=value text"
	case sp > 0 && (ext == ".cfg" || ext == ".txt"):
		return "console-style cfg (command value)"
	}
	return "text (" + ext + ")"
}

func describe(b *strings.Builder, format string, data []byte) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	switch format {
	case "binary", "UTF-16 text":
		h := data
		if len(h) > 16 {
			h = h[:16]
		}
		fmt.Fprintf(b, "    first bytes: % x\n", h)
	case "JSON":
		var v any
		if json.Unmarshal(data, &v) == nil {
			keys := 0
			walkJSON(b, "", v, &keys)
		}
	case "INI", "key=value text", "console-style cfg (command value)":
		section, keys := "", 0
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, ";") || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "//") {
				continue
			}
			if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
				section = strings.Trim(l, "[]") + "."
				continue
			}
			k, v, ok := strings.Cut(l, "=")
			if !ok {
				k, v, _ = strings.Cut(l, " ")
			}
			if keys++; keys > maxKeys {
				b.WriteString("    ...\n")
				return
			}
			line(b, section+strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}

func walkJSON(b *strings.Builder, path string, v any, keys *int) {
	if *keys > maxKeys {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		ks := make([]string, 0, len(t))
		for k := range t {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			p := k
			if path != "" {
				p = path + "." + k
			}
			walkJSON(b, p, t[k], keys)
		}
	case []any:
		if len(t) > 0 {
			fmt.Fprintf(b, "    %s: list of %d\n", path, len(t))
			walkJSON(b, path+"[0]", t[0], keys)
		} else {
			line(b, path, "[]")
		}
	default:
		*keys++
		if *keys > maxKeys {
			b.WriteString("    ...\n")
			return
		}
		raw, _ := json.Marshal(t)
		line(b, path, string(raw))
	}
}

func line(b *strings.Builder, key, val string) {
	mark := " "
	if settingHint.MatchString(key) {
		mark = "*"
	}
	fmt.Fprintf(b, "   %s %s = %s\n", mark, key, Mask(key, val))
}

// Mask hides values that could identify the player.
func Mask(key, val string) string {
	if secretKey.MatchString(key) {
		return "<masked>"
	}
	val = ipLike.ReplaceAllString(val, "x.x.x.x")
	if len(val) > 48 {
		return fmt.Sprintf("<%d chars masked>", len(val))
	}
	return val
}
