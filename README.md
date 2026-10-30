# Reclaimed: a launcher for Project Reclaimer and Reach Renaissance

Repository: https://github.com/tman-comps/Reclaimed-Launcher

An unofficial Windows launcher for two fan-made clients that run on top of
Steam's *Halo: The Master Chief Collection*. It installs each game from its
GitHub releases, checks the downloads against the release's SHA256SUMS,
shows versions and updates, and starts the games. It never modifies the
games' exes or MCC's files.

Not affiliated with Microsoft, Halo Studios, 343 Industries, Bungie, or either
game's developers. The built-in cover images are fan-made and include Halo
trademarks and artwork, which belong to Microsoft.

## Stack: Go + Wails v2

| Need | Why Wails |
|---|---|
| One exe, no installer | Go builds a static binary and Wails embeds the UI: `Reclaimed.exe` is about 13 MB. |
| Polished UI | The UI is HTML/CSS rendered by WebView2 (the Edge engine already on Windows 10/11), so the dark MCC-style layout is plain CSS. |
| Windows system access | `golang.org/x/sys/windows` gives the registry (finding Steam), process listing (is the game running?), known folders, and exe version info without cgo. |
| Safe file handling | Go's standard library covers SHA-256, zip extraction (with zip-slip checks), HTTP and atomic renames. |
| Building | Cross-compiles from Linux or Windows with `wails build`. |

Other options considered: Electron (exe of 150 MB or more), Tauri (similar
result to Wails, but cross-compiling Rust for MSVC is harder),
and .NET WPF (either a 70 MB self-contained exe or a .NET runtime install).

If WebView2 is missing (rare on Windows 10/11), the embedded bootstrapper
offers to install it. That's the one case where something is installed outside
the launcher's folder.

## Folder layout

Put `Reclaimed.exe` in its own folder, e.g. `C:\Games\Reclaimed\`. It refuses to work
from Program Files, and Reach Renaissance won't install if that folder is inside
MCC's folder. The launcher only writes inside its own folder:

```
Reclaimed\
  Reclaimed.exe
  games\
    reclaimer\             project-reclaimer-client-v<ver>.exe
    reach-renaissance\     unzipped release (Reach Renaissance.exe ...)
    .staging\              downloads in progress (cleaned up)
    .previous\<game>\      the version before the last update, for rollback
  data\
    state.json             what was installed, version and exe SHA-256
    launcher.log
    webview\               WebView2 cache (kept here, not in %APPDATA%)
  reports\                 settings reports (read-only scans, Phase 2)
  art\                     optional cover overrides (see below)
```

It only reads these locations: the Steam registry keys and `libraryfolders.vdf`,
`appmanifest_976730.acf`, the MCC folder,
`Documents\My Games\Project Reclaimer`, and `%LOCALAPPDATA%\ReachRenaissance`.

The one exception to "only writes inside its own folder" is when you press Save
under Settings > Game settings: the launcher then writes that game's own
settings file (`settings.json` or `settings.txt` in the folders above). It
refuses while the game runs or if the file changed since it was shown, keeps
every setting it doesn't know, and first copies the old file to
`data\backups\<game>\` (the last 20 are kept).

"Use an existing install..." links a copy you already have elsewhere. The
launcher then only launches that copy and never writes to its folder. Updates
for it come from the game's own updater.

## Cover art

Default covers for both games are built into the exe (`internal/art/defaults`).
To override one, put a file with the same name in `art\` next to the exe; delete
it to go back to the built-in cover. Sizes follow Steam's capsules:

| File | Size | Used by |
|---|---|---|
| `reclaimer-tall.<png/jpg/webp>` | 600×900 | Cards layout |
| `reclaimer-wide.<...>` | 920×430 | Banners layout |
| `reach-renaissance-tall.<...>` | 600×900 | Cards layout |
| `reach-renaissance-wide.<...>` | 920×430 | Banners layout |

The layout toggle (Cards / Banners) is in the title bar. Only files matching
these names are served to the UI (`internal/art`).

## How updates avoid fighting the games' own updaters

Both games update themselves: Reclaimer replaces its exe in place, and Reach
Renaissance installs signed updates when it starts. The launcher:

1. **Identifies versions by content, not filename.** It records the main exe's
   SHA-256 at install. If the exe changes and matches the latest release's
   SHA256SUMS entry (Reclaimer), the launcher accepts the new version. If it
   changes to something it can't map (Reach, whose sums cover only the zip), it
   shows "Updated in-game", stops suggesting updates and keeps its hands off.
2. **Only checks on start.** It never updates on its own; you press Update.
3. **Never writes while anything runs from the game's folder.** That includes
   the game's updater. It checks before the download and again right before
   swapping files.
4. **Swaps atomically.** Downloads go to `games\.staging` and must match
   SHA256SUMS (and GitHub's asset digest when present). Only then is the live
   copy moved to `.previous` and the new one moved in. If that fails, the old
   copy is restored.

## Building

Requires Go 1.26+ (an older Go downloads 1.26 automatically) and the Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1`).
The frontend is static files in `frontend/dist`, so no npm step is needed.

Wails v2.10.1 can't load packages with Go 1.27 or newer (`internal error: package
"context" without types ...`). If your Go is newer, build with the 1.26 toolchain;
Go downloads it automatically:

```powershell
$env:GOTOOLCHAIN = 'go1.26.10'
```

```bash
wails build -platform windows/amd64 -skipbindings -trimpath -ldflags "-s -w" -webview2 embed
```

Output: `build/bin/Reclaimed.exe`. It isn't code-signed, so SmartScreen will warn
the first time.

Tests (run on any OS; the Windows-only parts have stubs):

```bash
go test ./internal/...
```

To preview the UI in a browser with a fake backend (dev only, not shipped):

```bash
node frontend/dev/preview.js <screenshot-dir>
```

## Code map

| Path | What |
|---|---|
| `main.go`, `app.go` | Wails window and the methods the UI calls |
| `internal/steam`, `internal/vdf` | Steam library and MCC detection |
| `internal/release` | GitHub latest release, SHA256SUMS, version compare |
| `internal/install` | Verified download, safe unzip, atomic swap |
| `internal/games` | Game definitions, status and version detection, install/launch |
| `internal/proc` | Running-process check, launch, exe version info |
| `internal/discovery` | Reclaimer public server count |
| `internal/settings` | Each game's settings file: read, validate, back up, write (Phase 2) |
| `internal/report` | Read-only settings-folder scan |
| `internal/art` | Built-in cover images and `art\` overrides |
| `internal/servers` | **Phase 3 placeholder:** server browser and join interfaces, inactive |

## Roadmap

- **Phase 2: settings.** Settings > Game settings edits each game's video, graphics,
  mouse and audio settings (formats in `docs/settings-formats.md`). Its Sync tab
  compares the six settings both games share (field of view, vehicle field of
  view, frame limit, v-sync, window mode, volume) and copies them either way,
  only when you click.
- **Phase 3: server browser and join.** Blocked until each game documents a
  server-info query and a join method. Add `servers.Source`/`servers.Joiner`
  implementations and the Servers page will use them.
