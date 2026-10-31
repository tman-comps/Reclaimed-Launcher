package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func processImages() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []string
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		h, oerr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if oerr != nil {
			continue // protected/system processes
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
			out = append(out, windows.UTF16ToString(buf[:n]))
		}
		windows.CloseHandle(h)
	}
	return out, nil
}

// Launch starts exe detached from the launcher, with its own folder as the
// working directory, exactly as a double-click would.
func Launch(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// FileVersion reads the fixed version from an exe's version resource, or ""
// if it has none.
func FileVersion(path string) string {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var fixed *windows.VS_FIXEDFILEINFO
	var l uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &l); err != nil || fixed == nil {
		return ""
	}
	ms, ls := fixed.FileVersionMS, fixed.FileVersionLS
	if ms == 0 && ls == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}

// OpenFolder shows a folder in Explorer.
func OpenFolder(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}

// CreateShortcut writes a Windows shortcut (.lnk) at lnk that starts
// target in workDir, using Windows' own Shell COM object through a hidden
// PowerShell. The paths go in through environment variables, so nothing in
// them is read as script.
func CreateShortcut(lnk, target, workDir, description string) error {
	const script = `$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:RECLAIMED_LNK)
$s.TargetPath = $env:RECLAIMED_TARGET
$s.WorkingDirectory = $env:RECLAIMED_DIR
$s.IconLocation = $env:RECLAIMED_TARGET + ',0'
$s.Description = $env:RECLAIMED_DESC
$s.Save()`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(),
		"RECLAIMED_LNK="+lnk, "RECLAIMED_TARGET="+target, "RECLAIMED_DIR="+workDir, "RECLAIMED_DESC="+description)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(lnk); err != nil {
		return errors.New("Windows didn't create the shortcut")
	}
	return nil
}

// WaitExit waits until the process pid has exited, at most timeout.
func WaitExit(pid int, timeout time.Duration) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // already gone
	}
	defer windows.CloseHandle(h)
	windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
}
