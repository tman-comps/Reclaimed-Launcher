package steam

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// SteamRoot returns Steam's install folder from the registry, falling back to
// the default location.
func SteamRoot() string {
	try := []struct {
		root registry.Key
		path string
		name string
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"},
	}
	for _, t := range try {
		k, err := registry.OpenKey(t.root, t.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue(t.name)
		k.Close()
		if err == nil && v != "" {
			v = filepath.Clean(v)
			if st, err := os.Stat(v); err == nil && st.IsDir() {
				return v
			}
		}
	}
	def := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam")
	if st, err := os.Stat(def); err == nil && st.IsDir() {
		return def
	}
	return ""
}
