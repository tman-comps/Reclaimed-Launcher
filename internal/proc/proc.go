// Package proc launches game executables and tells whether anything is
// running from a game's folder, so the launcher never writes to a folder
// that a game or its own updater is using.
package proc

import (
	"path/filepath"
	"strings"
)

// underDir reports whether image is inside dir (case-insensitive, Windows paths).
func underDir(image, dir string) bool {
	if image == "" || dir == "" {
		return false
	}
	d := strings.ToLower(filepath.Clean(dir)) + string(filepath.Separator)
	return strings.HasPrefix(strings.ToLower(filepath.Clean(image)), d)
}

// RunningIn returns the executable paths of processes whose image lives
// under any of dirs, grouped by dir.
func RunningIn(dirs ...string) (map[string][]string, error) {
	images, err := processImages()
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, img := range images {
		for _, d := range dirs {
			if underDir(img, d) {
				out[d] = append(out[d], img)
			}
		}
	}
	return out, nil
}
