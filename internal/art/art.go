// Package art serves cover images. Built-in covers are embedded from
// defaults/; a file in <launcher>\art named <game id>-tall.<ext> (600x900)
// or <game id>-wide.<ext> (920x430) overrides the built-in one.
package art

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Kinds of artwork.
const (
	Tall = "tall"
	Wide = "wide"
)

var exts = []string{".webp", ".png", ".jpg", ".jpeg"}

var validName = regexp.MustCompile(`^[a-z0-9-]+-(tall|wide)\.(webp|png|jpe?g)$`)

const maxSize = 15 << 20

//go:embed defaults
var embedded embed.FS

var defaults, _ = fs.Sub(embedded, "defaults")

// Source says where a game's cover comes from: "custom", "built-in" or "".
func Source(dir, id, kind string) string {
	switch u := URL(dir, id, kind); {
	case u == "":
		return ""
	case strings.HasPrefix(u, "/art/default/"):
		return "built-in"
	default:
		return "custom"
	}
}

func builtin(id, kind string) string {
	for _, ext := range exts {
		if _, err := fs.Stat(defaults, id+"-"+kind+ext); err == nil {
			return id + "-" + kind + ext
		}
	}
	return ""
}

// URL returns the UI URL for a game's art (with an mtime cache-buster), or "".
func URL(dir, id, kind string) string {
	for _, ext := range exts {
		name := id + "-" + kind + ext
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() && st.Size() <= maxSize {
			return fmt.Sprintf("/art/%s?v=%d", name, st.ModTime().Unix())
		}
	}
	if name := builtin(id, kind); name != "" {
		return "/art/default/" + name
	}
	return ""
}

// Handler serves /art/<name> from dir. Anything else is 404, so the page
// can't be used to read other files.
func Handler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name, ok := strings.CutPrefix(r.URL.Path, "/art/default/"); ok {
			if !validName.MatchString(name) {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "max-age=86400")
			http.ServeFileFS(w, r, defaults, name)
			return
		}
		name, ok := strings.CutPrefix(r.URL.Path, "/art/")
		if !ok || !validName.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || st.Size() > maxSize {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, p)
	})
}
