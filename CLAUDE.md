# Reclaimed Launcher: notes for Claude

Windows desktop launcher (Go + Wails v2, single exe) for two closed-source,
releases-only fan clients that run on Steam's Halo: MCC:

- **Project Reclaimer** (Halo 3): github.com/ProjectReclaimer/project-reclaimer-releases,
  asset `project-reclaimer-client-v<ver>.exe` (older: `project-reclaimer-v<ver>.exe`,
  `project-reclaimer.exe`; never the `-dedicated-` one). Self-updates by replacing its exe.
  User data: `Documents\My Games\Project Reclaimer`. Server discovery:
  `GET http://148.251.153.44:49175/v3/discovery` (backup `23.159.176.140`), JSON
  `records` + `masters`, addresses only.
- **Reach Renaissance** (Halo: Reach): github.com/NETF1ix/reach-renaissance-releases,
  asset `ReachRenaissance-v<ver>.zip`, run `Reach Renaissance.exe`, must NOT be inside the
  MCC folder. Signed self-updater. Data/logs: `%LOCALAPPDATA%\ReachRenaissance`.

Repository: https://github.com/tman-comps/Reclaimed-Launcher (default branch `main`).

See README.md for the folder layout, update-conflict rules, and code map.

## Hard rules (from the owner)

- Never modify the games' exes or game files. Only install verified release assets.
- The launcher writes only inside its own folder (`games\`, `data\`, `reports\`, `art\`).
  **Ask the owner before anything that changes files outside it.**
- Every install must match the release's `SHA256SUMS.txt`; no sums = no install.
- Never write to a game folder while anything runs from it.
- Halo branding/art is allowed (owner's decision); default covers are in `internal/art/defaults`.

## Status

- Phase 1 done: MCC detection, install/update/verify, version detection by SHA-256
  (recognises the games' in-game updates), Play, Reclaimer server count, Cards/Banners
  layouts, cover art (built-in + `art\` overrides), custom icon.
- **Never run on real Windows until now.** It was built and tested on Linux (unit tests
  plus a browser preview with a mock backend). The first real run needs checking:
  MCC detection, a real install of both games, Play, the live server count, and the
  frameless window controls.
- Phase 2 settings pages are built (`internal/settings`, Settings > Game settings), from the
  formats in `docs/settings-formats.md`. Real saves worked for both games on 2026-10-30
  (Reclaimer then rewrote its file itself and kept the saved value). Saving writes the game's own settings
  file, the one owner-approved write outside the launcher folder: refused while the game
  runs or if the file changed since loading, unknown keys kept, backup to
  `data\backups\<game>\` first. Reclaimer's FOV uses Reach's list (70-120) and its frame
  limit 30-240 without "unlimited" (its encoding is unknown); the owner doesn't want mouse
  sensitivity/smoothing on the page. Settings sync between the games is NOT started.
  (Settings > "Export settings report" writes a read-only, masked report to `reports\`.) Writes must: back up first, refuse while the
  game runs, validate the format, and stop on unknown formats.
- Phase 3 (server browser + join) is blocked until the game devs document a server-info
  query and join method. Placeholder: `internal/servers`.

## Commands (Windows, PowerShell)

```powershell
go test ./internal/...
$env:GOTOOLCHAIN = 'go1.26.10'   # Wails v2.10.1 can't load packages built by Go 1.27+
wails dev            # live window with hot reload
wails build -skipbindings -trimpath -ldflags "-s -w" -webview2 embed   # -> build\bin\Reclaimed.exe
```

On Go 1.27+ without `GOTOOLCHAIN`, `wails build` fails with `internal error: package
"context" without types was imported from ...` (Wails v2.10.1's old `golang.org/x/tools`).

The frontend is static files in `frontend/dist` (no npm build). `frontend/dev/` has a
mock backend + Playwright preview script for screenshots, not shipped.
