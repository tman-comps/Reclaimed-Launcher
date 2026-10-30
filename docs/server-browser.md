# Phase 3 investigation: Project Reclaimer servers

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

## Per-server details: direct query (UDP, undocumented)

The in-game browser asks each server itself for its name, map and players
(Settings > "Server query batch size": "Maximum servers asked at once...
if many time out"; error text "The server described itself in a way this
build cannot show"). Servers didn't accept TCP or HTTP on their listed
port, so the query is UDP with the game's own format. It isn't documented
anywhere found. Getting it means capturing the game's browser traffic
(needs an admin `pktmon` session while the browser refreshes) or reading
it out of the exe.

## Joining

The game registers a link handler when it starts:
`HKCU\Software\Classes\reclaimer` -> `"<client exe>" "%1"`. Its website's
join page (`projectreclaimer.dev/join?server=host:port`) opens
`reclaimer://join/host:port` (from `assets/js/site.js`). The launcher can
start the installed client with that argument, which doesn't depend on the
handler being registered. Not tried yet: what happens when the game is
already running.

The game also has Direct Connect (type `host:port`) in its own browser.
