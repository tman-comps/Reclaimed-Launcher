# Phase 2 investigation: settings file formats

From the first settings report on a real PC (2026-10-30, Project Reclaimer
v0.9.23, Reach Renaissance v0.1.28), made with Settings > "Export settings
report". Option lists come from reading the games' own files without
changing anything: Reach Renaissance's menu script
(`themes\classic\start-menu.js`, plain JavaScript in its release) and the
text inside Project Reclaimer's exe. Nothing was written to either game.

## Project Reclaimer

Folder: `Documents\My Games\Project Reclaimer`

| File | Format | What it is | Phase 2 |
|---|---|---|---|
| `settings.json` | JSON, `"version": 1` | Video, audio, mouse, FOV | **Settings page** |
| `appearance.json` | JSON, `"version": 2` | Spartan/Elite armour, colours, emblem | Maybe later |
| `profile.json` | JSON, `"version": 1` | Nickname, identity public key, `protected_seed` (DPAPI-encrypted, `"protection": "windows-dpapi-user-v1"`) | **Never write** |
| `discovery/favorites.json` | JSON list of `"ip:port"` | Favourite servers | Phase 3 (server browser) |
| `discovery/peers.json` | JSON list of `{ip, origin, seen, failures}` | Known discovery peers | Read-only |
| `downloaded-mods.json`, `mod-digests.json` | JSON | Mod download cache (paths, sizes, SHA-256) | Read-only |
| `Game Modes/Project Reclaimer/*.json` | JSON | Custom game modes | Out of scope |
| `default-name-checked` | empty marker file | | Ignore |
| `Films/`, `Logs/` | | Saved films, logs | Ignore |

`settings.json` keys seen on this PC:

| Key | Example | Valid values |
|---|---|---|
| `field_of_view` | `85` | number (range unknown) |
| `vehicle_field_of_view` | `70` | number (range unknown) |
| `mouse_sensitivity` | `1.5` | number (range unknown) |
| `mouse_smoothing_ms` | `6` | number (range unknown) |
| `graphics.display_mode` | `"windowed"` | `windowed`, `borderless`, `exclusive_fullscreen` |
| `graphics.render_resolution` | `"r2160p"` | `r720p`, `r900p`, `r1080p`, `r1440p`, `r2160p` |
| `graphics.width`, `graphics.height` | `3840`, `2160` | numbers (window size) |
| `graphics.frame_limit` | `120` | number (range unknown) |
| `graphics.vsync` | `true` | bool |
| `audio.master_volume` | `1` | number, presumably 0 to 1 |
| `audio.muted`, `audio.mute_unfocused`, `audio.main_menu_music` | `false`/`true` | bool |
| `version` | `1` | format version |

The enum lists come from the exe's text, where each sits next to its
settings struct (`struct GraphicsSettings display_mode frame_limit
render_resolution vsync`). The exe also has `KeyboardMouseSettings`,
`LanguageSettings` (`subtitles`) and controller layouts (`southpaw`,
`legacy`, `legacy_southpaw`) that this PC's file doesn't contain, so the
file can lack whole sections. A settings page must not assume every key
exists, and must never drop sections it doesn't know. Number ranges aren't
in the text; clamp to what the game's own menu offers once that's known.

## Reach Renaissance

Folder: `%LOCALAPPDATA%\ReachRenaissance`

| File | Format | What it is | Phase 2 |
|---|---|---|---|
| `settings.txt` | `key = value` lines, no sections | All game settings, **only those changed from the defaults** | **Settings page** |
| `settings.txt.live` | 1 byte | Rewritten alongside `settings.txt` while the game runs; the menu also reads a `live` key of changes in progress | Treat as "in use"; never write |
| `armory-appearance.txt` | `key = value`; hex colours, part names, emblem as `16.44.0.c23b33.2fa59a.6a5aa8` | Armour and colours | Maybe later |
| `armor.txt` | `key = value` where many keys are hex offsets (`44 = 5a`) plus `elite.*` | Raw armour data | **Never write** |
| `player_id.txt` | 18-byte text | Player identity | **Never write** |
| `music/*.wav` | | Menu music copied from the game | Ignore |
| `runs/` | | Per-run logs | Ignore |

This PC's file only has `display = borderless`, `music = reach`,
`name = TS251` and `theme = title`, because everything else is at its
default. The menu script lists every setting, its default and its options
(all values are strings):

| Key | Default | Options |
|---|---|---|
| `display` | `windowed` | `windowed`, `borderless` |
| `resolution` | `auto` | `auto`, `1280x720`, `1600x900`, `1920x1080`, `2560x1080`, `2560x1440`, `3440x1440`, `3840x2160` |
| `fov`, `fov.vehicle` | `78` | `70`, `78`, `85`, `90`, `95`, `100`, `110`, `120` |
| `fps` | `0` (unlimited) | `0`, `30`, `60`, `90`, `120`, `144`, `165`, `240` |
| `vsync` | `off` | `off`, `on` |
| `gamma`, `ui.gamma` | `5` | `1` to `10` in steps of `0.5` |
| `gfx.preset` | `enhanced` | `performance`, `original`, `enhanced` (`custom` when the rows match none) |
| `gfx.details`, `gfx.effects`, `gfx.lighting`, `gfx.shadows`, `gfx.filtering`, `gfx.water` | `high` | `low`, `normal`, `high` |
| `gfx.aa`, `gfx.motionblur` | `on` | `off`, `on` |
| `mouse` (sensitivity, %) | `100` | `50`, `60`, `70`, `80`, `90`, `100`, `110`, `125`, `150`, `175`, `200`, `250`, `300` |
| `mouse.smoothing` | `off` | `off`, `on` |
| `crosshair` | `centered` | `lowered`, `centered` |
| `camera.distance` | `100` | `50` to `300` |
| `volume.game`, `volume.music`, `volume.effects`, `volume.menu`, `volume.menumusic` | `100` | `0` to `100` in steps of 10 |
| `music` | `reach` | `reach`, `track`, `playlist`, `off` |
| `subtitles` | `automatic` | `automatic`, `on`, `off` |
| `pad.*`, `bind.*`, `chat.*`, `voice.*`, `camera.toggle` and others | | Controller, keybinds, chat, voice: leave alone |

Changing `gfx.preset` also sets every `gfx.*` row, and changing a `gfx.*`
row sets `gfx.preset` to what the rows add up to. A settings page must
keep them consistent the same way.

## What this means for Phase 2

- **Reclaimer settings page**: build it on `settings.json`. Refuse unless
  `version` is 1. Change only the known keys, keep every other key and
  section exactly as it was, back up first, and refuse while the game runs.
- **Reach settings page**: edit `settings.txt` lines in place so order,
  unknown lines and formatting survive. Write only values from the lists
  above. Leave a key out (rather than writing its default) when it isn't
  already in the file. Refuse while the game runs or `settings.txt.live`
  looks active.
- **Sync between games** is possible for:
  - FOV and vehicle/third-person FOV, snapped to Reach's list.
  - Frame limit (Reclaimer number vs Reach list; `0` = unlimited in Reach).
  - V-sync.
  - Display mode: Reach has no `exclusive_fullscreen`.
  - Resolution: different scales (`r1080p` vs `1920x1080`), mapping needed.
  - Not mouse sensitivity: Reclaimer's `1.5` and Reach's `100` (%) use
    different units with no known conversion.
- Still unknown: Reclaimer's number ranges (FOV, frame limit, sensitivity,
  smoothing). Its in-game settings menu shows them.

Both settings pages are built in `internal/settings` as described above.

## Report scanner issues found (fixed)

- Reach Renaissance keeps a WebView2 cache in
  `games\reach-renaissance\bin\menu-overlay.exe.WebView2\`. The scan of the
  game folder spent its 400-file limit there and stopped before the rest.
  `*.WebView2` folders are now skipped.
- `README.txt` files were parsed as `key=value` text; documentation files
  are now listed but not read.
- The `*` settings hint matched `key` inside `public_key`.
