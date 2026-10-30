package paths

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Documents is the user's (possibly OneDrive-redirected) Documents folder.
func Documents() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil {
		return p
	}
	return filepath.Join(os.Getenv("USERPROFILE"), "Documents")
}

// LocalAppData is %LOCALAPPDATA%.
func LocalAppData() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0); err == nil {
		return p
	}
	return os.Getenv("LOCALAPPDATA")
}
