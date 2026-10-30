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
- Phase 2 (settings pages/sync) is NOT started: first inspect each game's settings files on
  the PC (Settings > "Export settings report" writes a read-only, masked report to
  `reports\`), report the formats, then build. Writes must: back up first, refuse while the
  game runs, validate the format, and stop on unknown formats.
- Phase 3 (server browser + join) is blocked until the game devs document a server-info
  query and join method. Placeholder: `internal/servers`.

## Commands (Windows, PowerShell)

```powershell
go test ./internal/...
wails dev            # live window with hot reload
wails build -trimpath -ldflags "-s -w" -webview2 embed   # -> build\bin\Reclaimed.exe
```

The frontend is static files in `frontend/dist` (no npm build). `frontend/dev/` has a
mock backend + Playwright preview script for screenshots, not shipped.
