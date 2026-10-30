# Phase 2 investigation: settings file formats

From the first settings report on a real PC (2026-10-30, Project Reclaimer
v0.9.23, Reach Renaissance v0.1.28), made with Settings > "Export settings
report". Nothing was written to either game's files.

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

`settings.json` keys seen (all values as the game wrote them):

| Key | Example | Type |
|---|---|---|
| `field_of_view` | `85` | number |
| `vehicle_field_of_view` | `70` | number |
| `mouse_sensitivity` | `1.5` | number |
| `mouse_smoothing_ms` | `6` | number |
| `graphics.display_mode` | `"windowed"` | string (other values unknown) |
| `graphics.width`, `graphics.height` | `3840`, `2160` | number |
| `graphics.render_resolution` | `"r2160p"` | string enum (other values unknown) |
| `graphics.frame_limit` | `120` | number |
| `graphics.vsync` | `true` | bool |
| `audio.master_volume` | `1` | number, presumably 0 to 1 |
| `audio.muted`, `audio.mute_unfocused`, `audio.main_menu_music` | `false`/`true` | bool |
| `version` | `1` | format version |

No keybinds were found in any file. The file was last modified 2026-09-30,
so the game only rewrites it when a setting changes.

## Reach Renaissance

Folder: `%LOCALAPPDATA%\ReachRenaissance`

| File | Format | What it is | Phase 2 |
|---|---|---|---|
| `settings.txt` | `key = value` lines, no sections | Launcher-menu settings | **Settings page** |
| `settings.txt.live` | 1 byte | Rewritten alongside `settings.txt` while the game runs; probably a live/lock marker | Treat as "in use"; never write |
| `armory-appearance.txt` | `key = value`; hex colours, part names, emblem as `16.44.0.c23b33.2fa59a.6a5aa8` | Armour and colours | Maybe later |
| `armor.txt` | `key = value` where many keys are hex offsets (`44 = 5a`) plus `elite.*` | Raw armour data | **Never write** |
| `player_id.txt` | 18-byte text | Player identity | **Never write** |
| `music/*.wav` | | Menu music copied from the game | Ignore |
| `runs/` | | Per-run logs | Ignore |

`settings.txt` keys seen: `display` (`borderless`), `music` (`reach`),
`name` (`TS251`), `theme` (`title`). Other values for each are unknown.

FOV, sensitivity, resolution and keybinds are **not** in Reach
Renaissance's own files. They most likely live in MCC's own config, which
the launcher doesn't read today; checking it needs the owner's OK first.

## What this means for Phase 2

- **Reclaimer settings page**: build it on `settings.json`. Refuse unless
  `version` is 1. Change only the known keys, keep every other key exactly
  as it was, back up first, and refuse while the game runs.
- **Reach settings page**: small (display mode, music, theme). Edit lines
  in place so order, unknown lines and formatting survive. Refuse while the
  game runs or `settings.txt.live` looks active.
- **Sync between games**: only the display mode and the player name exist
  in both. FOV and sensitivity sync would need MCC's config for Reach.
- Valid values for the string enums (`display_mode`, `render_resolution`,
  Reach `display`, `music`, `theme`) still need to be found, by changing
  them in each game's menu and exporting another report.

## Report scanner issues found

- Reach Renaissance keeps a WebView2 cache in
  `games\reach-renaissance\bin\menu-overlay.exe.WebView2\`. The scan of the
  game folder spent its 400-file limit there and stopped before the rest.
  Skip `*.WebView2` folders.
- `README.txt` files are parsed as `key=value` text, which is noise.
- The `*` settings hint matches `key` inside `public_key`.
