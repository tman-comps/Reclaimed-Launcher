# Phase 3: Project Reclaimer servers

Found 2026-10-30 from the game's settings screen (Server discovery and
Stats service), its exe's own text, and its website. Only read-only
requests were made.

## Live server list: discovery masters

The game's default masters (Settings > Server discovery):

```
http://127.0.0.1:49175        the game's own node, while it runs
http://148.251.153.44:49175
http://23.159.176.140:49175
```

`GET /v3/discovery` returns `records` and `masters`. Each record is
`{"record": {"key", "address", "issued", "expires", "origin"?}, "signature"}`:

- `address` is the server's `host:port` (an IP or a host name; ports seen
  49176 to 50208). 374 distinct addresses on 2026-10-30.
- Records live 180 seconds (`expires - issued`), so the list is live.
- `origin` (optional) is that server's own discovery node, `http://IP:49175`.
- `masters` lists other masters (40 to 60 of them), which gossip records.

The launcher already uses this for its server count (`internal/discovery`).

## Names and recent games: stats service

`http://148.251.153.44:8790` (Settings > Stats service). Paths from the
exe: `/health`, `/v1/servers`, `/v1/servers/{id}`, `/v1/matches`,
`/v1/players/...`, `/v1/unlocks/lookup`.

- `GET /health`: lifetime totals, e.g. `{"matches":31946,"players":62073,"status":"ok",...}`.
- `GET /v1/servers`: 50 servers that report to the stats service:
  `{"key","name","matches","added_at"}`. No address, and `key` is **not**
  the discovery record's key, so names can't be joined to live addresses.
- `GET /v1/matches`: the last 25 finished matches:
  `{"server_name","map","map_title","mode","variant","team_game","ranked","started_at","ended_at","players"}`.
  No server address.
- `GET /v1/servers/{id}` with a key gave 404 (id format unknown).

## Per-server details: status query (QUIC + framed JSON)

The in-game browser asks each server itself for its name, map and players
(Settings > "Server query batch size"). Worked out on 2026-10-30 from a
`pktmon` capture of the game's browser and a logging QUIC relay the game
was pointed at with Direct Connect:

- **Transport:** QUIC v1 to the record's `host:port`, TLS server name
  `reclaimer`, **no ALPN**. Servers present a throwaway self-signed
  certificate (`CN=rcgen self signed cert`, DNS `reclaimer`); the game is
  built on quinn + rustls.
- **One connection per question**, one bidirectional stream.
- **Framing:** 4-byte big-endian length, then UTF-8 JSON, both ways.
- **Request:** `{"status":2}`. The client closes its send side.
- **Reply:** one frame, then the server closes:

```json
{"name":"DaBois | Hugegrass 120 Player (Updated)","host_name":"Juices Server",
 "map":"Hugegrass · Custom","mode":"Capture the Flag","players":99,"capacity":120,
 "ready":true,"password_required":false,
 "mods":[{"id":"3a88…","title":"Hugegrass","maps":[{"stem":"…","digest":"…"}],"size":2118599548,"shared":true}],
 "version":"0.9.23","variant":"62Player CTF","map_variant":"Hugegrass HugeTeamBattle",
 "player_names":["its_Grippy","…"],"blocks_vpn":true,"max_ping":…}
```

About 2 KB for a full 120-player server. On 2026-10-30, 322 of 374 listed
servers answered within 3 seconds (48 asked at once, 6.7 s in all).

A join is a different request on the same kind of connection
(`{"nickname":…,"public_key":[…]}` answered by a session, peer and
nonce): it sends the player's identity, so **the launcher never sends
it**. Joining is left to the game (below).

Implemented in `internal/servers` (`Querier.Query`, `Querier.Browse`).

## Joining

The game registers a link handler when it starts:
`HKCU\Software\Classes\reclaimer` -> `"<client exe>" "%1"`. Its website's
join page (`projectreclaimer.dev/join?server=host:port`) opens
`reclaimer://join/host:port` (from `assets/js/site.js`). The launcher
starts the installed client with that argument (`Manager.Join`), which
doesn't depend on the handler being registered. A running game takes the
link over itself (exe text: "The running game joins …"; "The running game
did not take the join link; starting another.").

The game also has Direct Connect (type `host:port`) in its own browser.
