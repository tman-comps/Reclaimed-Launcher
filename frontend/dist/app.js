"use strict";
// Reclaimed UI. Talks to the Go side through Wails bindings (window.go.main.App)
// and events (window.runtime). Text from GitHub is always escaped.

const $ = (s, el = document) => el.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

let api, rt;
let snap = null;
const progress = {}; // id -> {phase, done, total}
let openMenu = null;
let layout = "cards"; // "cards" (portrait, MCC-style) or "banners" (wide header)
try { layout = localStorage.getItem("reclaimed.layout") || layout; } catch {}

const ICON = {
  play: '<svg viewBox="0 0 16 16"><path d="M4 2.5v11l9-5.5z"/></svg>',
  down: '<svg viewBox="0 0 16 16"><path d="M7 1h2v8.2l2.8-2.8 1.4 1.4L8 13 2.8 7.8l1.4-1.4L7 9.2zM2 14h12v1.5H2z"/></svg>',
  more: '<svg viewBox="0 0 16 16"><circle cx="3" cy="8" r="1.5"/><circle cx="8" cy="8" r="1.5"/><circle cx="13" cy="8" r="1.5"/></svg>',
  stop: '<svg viewBox="0 0 16 16"><rect x="3.5" y="3.5" width="9" height="9"/></svg>',
};

// ---------------------------------------------------------------- helpers
function fmtBytes(n) {
  if (!(n > 0)) return "";
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(i > 1 ? 1 : 0)} ${u[i]}`;
}
function ago(t) {
  const d = new Date(t);
  if (!t || isNaN(d) || d.getFullYear() < 2000) return "";
  const s = Math.round((Date.now() - d) / 1000);
  if (s < 45) return "just now";
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return d.toLocaleDateString();
}
function toast(msg, kind = "") {
  const el = document.createElement("div");
  el.className = `toast ${kind}`;
  el.textContent = msg;
  $("#toasts").append(el);
  // Errors stay until clicked so they can't be missed.
  if (kind === "bad") {
    el.title = "Click to dismiss";
    el.style.cursor = "pointer";
    el.addEventListener("click", () => el.remove());
  } else setTimeout(() => el.remove(), 4500);
}
async function call(fn, ...args) {
  try {
    return await api[fn](...args);
  } catch (e) {
    toast(String(e?.message ?? e), "bad");
    throw e;
  }
}
function showModal(title, html) {
  $("#modalTitle").textContent = title;
  $("#modalBody").innerHTML = html;
  $("#modal").hidden = false;
}
// Tiny, safe markdown: headings, lists, bold, inline code. Links become text.
function md(src) {
  const inline = (s) => esc(s)
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1");
  let out = "", inList = false;
  for (const raw of String(src || "").split(/\r?\n/)) {
    const line = raw.trimEnd();
    const li = line.match(/^\s*[-*]\s+(.*)/);
    if (li) { if (!inList) { out += "<ul>"; inList = true; } out += `<li>${inline(li[1])}</li>`; continue; }
    if (inList) { out += "</ul>"; inList = false; }
    const h = line.match(/^#{1,6}\s+(.*)/);
    if (h) out += `<h4>${inline(h[1])}</h4>`;
    else if (line.trim()) out += `<p>${inline(line)}</p>`;
  }
  if (inList) out += "</ul>";
  return out || "<p>No release notes.</p>";
}

// ---------------------------------------------------------------- banners
let selfUpdateText = "Downloading…";
const TIP_KEY = "reclaimed.shortcutTipHidden";
function tipHidden() { try { return localStorage.getItem(TIP_KEY) === "1"; } catch { return false; } }
function hideTip() { try { localStorage.setItem(TIP_KEY, "1"); } catch {} renderBanners(); }
function renderBanners() {
  const b = [];
  const L = snap.launcher, m = snap.mcc;
  if (L.problem) b.push(`<div class="banner bad"><b>Launcher folder:</b> ${esc(L.problem)}</div>`);
  const U = L.update || {};
  if (L.updating) b.push(`<div class="banner"><span><b>Updating Reclaimed…</b> ${esc(selfUpdateText)} It restarts by itself when it's done.</span></div>`);
  else if (U.available) b.push(`<div class="banner"><span><b>Reclaimed ${esc(U.latest)} is available.</b> You have ${esc(L.version)}. Your games and settings stay as they are.</span><span class="spacer"></span><button class="btn small" data-act="launcher-notes">What's new</button><button class="btn small primary" data-act="update-launcher">Update now</button></div>`);
  if (!tipHidden()) b.push(`<div class="banner"><span><b>Tip:</b> start Reclaimed from your desktop or your Steam library.</span><span class="spacer"></span><button class="btn small" data-act="desktop-shortcut">Desktop shortcut</button><button class="btn small" data-act="add-steam">Add to Steam</button><button class="btn small" data-act="tip-hide" title="Hide this tip (the buttons are also under Settings › Launcher)" aria-label="Hide tip">✕</button></div>`);
  if (!m.found) {
    b.push(`<div class="banner bad"><span><b>MCC not found.</b> ${m.steamFound ? "Steam is installed, but Halo: The Master Chief Collection wasn't found in any Steam library." : "Steam wasn't found."} Both games need the Steam version of MCC.</span><span class="spacer"></span><button class="btn small" data-act="locate-mcc">Locate MCC folder</button><button class="btn small" data-act="redetect">Retry</button></div>`);
  } else {
    const missing = m.components.filter((c) => !c.installed);
    if (missing.length) b.push(`<div class="banner"><span><b>${missing.map((c) => esc(c.name)).join(" and ")} ${missing.length > 1 ? "aren't" : "isn't"} installed in MCC.</b> Install in Steam: right-click MCC › Manage › DLC.</span><span class="spacer"></span><button class="btn small" data-act="redetect">Check again</button></div>`);
  }
  $("#banners").innerHTML = b.join("");
}

function renderPill() {
  const m = snap.mcc;
  let cls = "bad", text = "MCC not found";
  if (m.found) {
    const n = m.components.filter((c) => c.installed).length;
    cls = n === m.components.length ? "ok" : "warn";
    text = n === m.components.length ? "MCC ready" : "MCC: game missing";
  }
  $("#mccPill").innerHTML = `<span class="dot ${cls}"></span>${text}`;
}

// ---------------------------------------------------------------- tiles
function tileState(g) {
  if (g.busy || progress[g.id]) return "busy";
  if (!g.installed) return "absent";
  return "installed";
}

function versionChip(g) {
  if (!g.installed) return `<span class="chip">Not installed</span>`;
  if (g.version) {
    const src = { file: " (file)", release: "" }[g.versionSource] ?? "";
    return `<span class="chip">v${esc(g.version)}${src}</span>`;
  }
  return `<span class="chip" title="The game updated itself to a build the launcher can't map to a release.">${g.updatedInGame ? "Updated in-game" : "Unknown version"}</span>`;
}

function tileHTML(g) {
  const st = tileState(g);
  const p = progress[g.id];
  const chips = [versionChip(g)];
  if (g.running) chips.push(`<span class="chip ok"><span class="dot ok live"></span>Running</span>`);
  if (g.updateAvailable && st !== "busy") chips.push(`<span class="chip warn">Update v${esc(g.latest)}</span>`);
  else if (g.installed && g.latest && !g.updateAvailable && g.versionSource !== "" ) chips.push(`<span class="chip ok">Up to date</span>`);
  if (g.linked) chips.push(`<span class="chip accent" title="${esc(g.dir)}">Linked folder</span>`);
  chips.push(serverChip(g.id));
  if (g.id === "reclaimer") chips.push(friendsChip());

  const top = [];
  if (!g.requirementOk) top.push(`<span class="chip bad">Needs ${esc(g.needsName)}</span>`);
  if (g.checkError && !g.latest) top.push(`<span class="chip warn" title="${esc(g.checkError)}">Update check failed</span>`);

  let note = "";
  if (g.blocker) note = `<p class="note warn">${esc(g.blocker)}</p>`;
  else if (!g.requirementOk) note = `<p class="note warn">${esc(g.requirementMsg)}</p>`;
  else if (g.checkError && g.latest) note = `<p class="note">Couldn't re-check for updates: ${esc(g.checkError)}</p>`;
  else if (g.updatedInGame) note = `<p class="note">The game updated itself. It keeps its own copy current, so the launcher won't overwrite it.</p>`;

  let prog = "";
  if (st === "busy") {
    const total = p?.total > 0 ? p.total : 0;
    const pct = total ? Math.min(100, (p.done / total) * 100) : 0;
    const label = { check: "Checking release…", download: "Downloading", verify: "Verifying SHA-256…", install: "Installing…", done: "Finishing…" }[p?.phase] ?? (g.busy || "Working…");
    prog = `<div class="progress"><div class="bar"><div class="fill ${total ? "" : "indet"}" style="width:${pct}%"></div></div>
      <div class="ptext"><span>${esc(label)}</span><span>${total ? `${fmtBytes(p.done)} / ${fmtBytes(total)}` : ""}</span></div></div>`;
  }

  let primary;
  const blocked = !!g.blocker || !!snap.launcher.problem;
  if (st === "busy") {
    primary = `<button class="btn" data-act="cancel" data-id="${g.id}">${ICON.stop}Cancel</button>`;
  } else if (st === "absent") {
    const size = g.latestSize ? ` · ${fmtBytes(g.latestSize)}` : "";
    primary = `<button class="btn primary" data-act="install" data-id="${g.id}" ${blocked || !g.latest ? "disabled" : ""} title="${g.latest ? `v${esc(g.latest)}${size}` : "Waiting for release info"}">${ICON.down}Install</button>`;
  } else {
    primary = `<button class="btn primary" data-act="play" data-id="${g.id}" ${g.running ? "disabled" : ""}>${ICON.play}${g.running ? "Running" : "Play"}</button>`;
    if (g.updateAvailable) primary += `<button class="btn" data-act="install" data-id="${g.id}" ${g.running || blocked ? "disabled" : ""} title="${g.running ? "Close the game to update" : `Update to v${esc(g.latest)}`}">${ICON.down}<span class="lbl">Update</span></button>`;
  }

  const glyph = g.id === "reclaimer" ? "III" : "R";
  // Cards prefer portrait art, banners prefer wide; fall back to the other
  // image, then to the built-in CSS artwork.
  const img = layout === "cards" ? (g.artTall || g.artWide) : (g.artWide || g.artTall);
  const media = img
    ? `<img class="cover" src="${esc(img)}" alt="" draggable="false" style="object-position:${img === g.artTall || layout === "banners" ? "center" : "left center"}">`
    : `<div class="art"><div class="glyph">${glyph}</div></div>`;
  const blur = img ? `<div class="blur" style="background-image:url('${esc(g.artWide || img)}')"></div>` : "";
  return `<article class="tile${img ? " has-art" : ""}" data-id="${g.id}">
    ${blur}
    <div class="media">${media}</div>
    <div class="tile-top">${top.join("")}</div>
    <div class="tile-body">
      <div class="kicker">${esc(g.tagline)}</div>
      <h2>${esc(g.name)}</h2>
      <div class="meta">${chips.join("")}</div>
      ${layout === "banners" ? whatsNew(g) : ""}${note}${prog}
      <div class="actions">${primary}
        <div class="menu-wrap"><button class="btn ghost" data-act="menu" data-id="${g.id}" aria-label="More">${ICON.more}</button></div>
      </div>
    </div>
  </article>`;
}

// A few plain-text lines from the latest release notes.
function whatsNew(g) {
  const lines = String(g.latestNotes || "").split(/\r?\n/)
    .map((l) => l.replace(/^#+\s*|^\s*[-*]\s+/, "").replace(/\*\*|`/g, "").replace(/\[([^\]]+)\]\([^)]+\)/g, "$1").trim())
    .filter((l) => l && !/^(changes|changelog|what'?s new|release notes)[:.]?$/i.test(l));
  if (!lines.length) return "";
  return `<button class="whatsnew" data-act="notes" data-id="${g.id}" title="Read full release notes">
    <span class="wn-head">What's new in v${esc(g.latest)}</span>
    <span class="wn-text">${lines.slice(0, 4).map(esc).join(" · ")}</span></button>`;
}

const plural = (n, word) => `${n.toLocaleString()} ${word}${n === 1 ? "" : "s"}`;

// Servers and players online. Halo 3's players come from asking every server
// (every few minutes); until then it shows the discovery service's count.
function serverChip(id) {
  const p = snap.population?.[id];
  if (p) return `<span class="chip accent" title="Public servers that answered · updated ${esc(ago(p.at))}"><span class="dot ok live"></span>${plural(p.servers, "server")} online · ${plural(p.players, "player")}</span>`;
  if (id !== "reclaimer") return `<span class="chip"><span class="dot"></span>Servers…</span>`;
  const d = snap.discovery;
  if (!d || !d.fetchedAt || new Date(d.fetchedAt).getFullYear() < 2000) return `<span class="chip"><span class="dot"></span>Servers…</span>`;
  if (!d.ok) return `<span class="chip warn" title="${esc(d.error)}"><span class="dot warn"></span>Server list offline</span>`;
  return `<span class="chip accent" title="Public servers listed by the discovery service · ${esc(ago(d.fetchedAt))}"><span class="dot ok live"></span>${d.servers} server${d.servers === 1 ? "" : "s"} online</span>`;
}

function menuHTML(g) {
  const busy = tileState(g) === "busy";
  const items = [];
  items.push(`<button data-act="open-folder" ${g.installed ? "" : "disabled"}>Open game folder</button>`);
  items.push(`<button data-act="open-data" ${g.userDataExists ? "" : "disabled"} title="${esc(g.userDataDir)}">Open settings &amp; logs folder</button>`);
  items.push(`<button data-act="notes" ${g.latest ? "" : "disabled"}>Release notes${g.latest ? ` (v${esc(g.latest)})` : ""}</button>`);
  items.push(`<button data-act="release-page" ${g.latestUrl ? "" : "disabled"}>Open release page</button>`);
  items.push("<hr>");
  if (g.linked) {
    items.push(`<button data-act="unlink">Stop using linked folder</button>`);
  } else {
    if (g.installed) items.push(`<button data-act="install" ${busy || g.running || !g.latest || g.blocker ? "disabled" : ""}>Reinstall latest${g.latest ? ` (v${esc(g.latest)})` : ""}</button>`);
    if (!g.installed) items.push(`<button data-act="link" ${busy ? "disabled" : ""}>Use an existing install…</button>`);
  }
  items.push(`<button data-act="about">About updates</button>`);
  return `<div class="menu" data-id="${g.id}">${items.join("")}</div>`;
}

// The menu floats above everything so tile edges can't clip it.
function placeMenu() {
  document.querySelectorAll("body > .menu").forEach((m) => m.remove());
  if (!openMenu) return;
  const g = snap.games.find((x) => x.id === openMenu);
  const btn = $(`.tile[data-id="${openMenu}"] [data-act=menu]`);
  if (!g || !btn) return;
  document.body.insertAdjacentHTML("beforeend", menuHTML(g));
  const m = document.body.lastElementChild, r = btn.getBoundingClientRect();
  const left = Math.max(8, Math.min(r.right - m.offsetWidth, innerWidth - m.offsetWidth - 8));
  const top = r.top - m.offsetHeight - 6 >= 8 ? r.top - m.offsetHeight - 6 : r.bottom + 6;
  Object.assign(m.style, { left: `${left}px`, top: `${top}px` });
}

// Size portrait cards so card + details fill the available height.
function fitCards() {
  const el = $("#tiles");
  if (layout !== "cards" || !el.offsetParent) return;
  const cs = getComputedStyle(el);
  const padY = parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom);
  const padX = parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight);
  const n = Math.max(1, snap.games.length);
  const maxW = (el.clientWidth - padX - 26 * (n - 1)) / n;
  for (let i = 0; i < 2; i++) { // body height depends on width; settle twice
    const body = Math.max(...[...el.querySelectorAll(".tile-body")].map((b) => b.offsetHeight), 150);
    const w = Math.max(190, Math.min(maxW, ((el.clientHeight - padY - body - 2) * 2) / 3));
    el.style.setProperty("--cap-w", `${Math.floor(w)}px`);
    el.classList.toggle("compact", w < 270);
  }
}

function setLayout(l) {
  layout = l;
  try { localStorage.setItem("reclaimed.layout", l); } catch {}
  document.querySelectorAll("#layoutSeg button").forEach((b) => b.classList.toggle("on", b.dataset.layout === l));
  renderTiles();
}

function renderTiles() {
  // Keep an open menu across re-renders.
  const el = $("#tiles");
  el.className = `tiles ${layout}`;
  el.innerHTML = snap.games.map(tileHTML).join("");
  fitCards();
  placeMenu();
}

// ---------------------------------------------------------------- pages
// Project Reclaimer's server browser. Built once, so typing, ticks and scroll
// survive redraws; only the rows and the status line are redrawn.
const sb = { el: null, list: null, q: "", hideEmpty: false, hideFull: false, hidePw: false, onlyFriends: false, open: null, friends: [] };

// ---- friends: player names to look for on Project Reclaimer servers.
const friendKey = (n) => n.trim().toLowerCase();
const isFriend = (n) => sb.friends.some((f) => friendKey(f) === friendKey(n));
// Friends playing right now, with the server each is on.
function friendsOnline() {
  const out = [], seen = new Set(); // a name on two servers counts once (lists are sorted busiest first)
  for (const s of sb.list?.servers || []) for (const n of s.playerNames || []) {
    if (isFriend(n) && !seen.has(friendKey(n))) { seen.add(friendKey(n)); out.push({ name: n, server: s }); }
  }
  return out;
}
async function setFriends(list) {
  sb.friends = (await call("SetFriends", list).catch(() => null)) ?? sb.friends;
  if (sb.el) { sbFriends(); sbRows(); }
  renderTiles();
}
function friendsChip() {
  const n = sb.friends.length ? friendsOnline().length : 0;
  return n ? `<span class="chip ok" title="${esc(friendsOnline().map((f) => `${f.name}: ${f.server.name}`).join("\n"))}">★ ${n} friend${n === 1 ? "" : "s"} playing</span>` : "";
}

function renderServers() {
  if (!sb.el) {
    sb.el = document.createElement("div");
    sb.el.innerHTML = `
      <div class="card">
        <div class="row"><h3 style="flex:1">Project Reclaimer</h3><span class="muted" id="sbStatus"></span>
          <button class="btn small" data-act="sb-refresh" id="sbRefresh">Refresh</button></div>
        <div id="sbFriends" class="sb-friends"></div>
        <div class="row sb-tools">
          <input id="sbSearch" type="search" placeholder="Search server, map, mode or player" autocomplete="off" spellcheck="false">
          <label><input type="checkbox" id="sbEmpty"> Hide empty</label>
          <label><input type="checkbox" id="sbFull"> Hide full</label>
          <label><input type="checkbox" id="sbPw"> Hide password-protected</label>
          <label><input type="checkbox" id="sbOnlyFriends"> Only with friends</label>
        </div>
        <div class="sb-list">
          <div class="sb-row sb-head"><div>Server</div><div>Map</div><div>Mode</div><div>Players</div><div>Ping</div><div></div></div>
          <div id="sbRows"></div>
        </div>
        <div class="row sb-direct">
          <input id="sbAddr" placeholder="Direct connect: host:port" autocomplete="off" spellcheck="false">
          <button class="btn small" data-act="sb-direct">Join</button>
          <span class="muted">Joining starts Project Reclaimer (or hands the server to it if it's open).</span>
        </div>
      </div>
      <div class="card">
        <h3>Reach Renaissance</h3>
        <p id="sbReach"></p>
      </div>`;
    $("#sbSearch", sb.el).oninput = (e) => { sb.q = e.target.value.trim().toLowerCase(); sbRows(); };
    for (const [id, k] of [["sbEmpty", "hideEmpty"], ["sbFull", "hideFull"], ["sbPw", "hidePw"], ["sbOnlyFriends", "onlyFriends"]]) {
      $(`#${id}`, sb.el).onchange = (e) => { sb[k] = e.target.checked; sbRows(); };
    }
    $("#sbAddr", sb.el).onkeydown = (e) => { if (e.key === "Enter") sbJoin(e.target.value.trim()); };
    $("#sbRows", sb.el).addEventListener("click", (e) => {
      const row = e.target.closest(".sb-row[data-addr]");
      if (!row || e.target.closest("button")) return;
      sb.open = sb.open === row.dataset.addr ? null : row.dataset.addr;
      sbRows();
    });
    $("#serversBody").replaceChildren(sb.el);
    sbRows();
    sbFriends();
  }
  const rp = snap.population?.["reach-renaissance"];
  $("#sbReach", sb.el).textContent = (rp ? `${plural(rp.servers, "server")} online · ${plural(rp.players, "player")}. ` : "") +
    "Join them from Reach Renaissance's own server browser: it has no way for the launcher to join for you.";
  sbStatus();
}

// Friends online (with Join) and the saved list (with remove), plus Add.
function sbFriends() {
  const box = $("#sbFriends", sb.el);
  const on = friendsOnline(), onNames = new Set(on.map((f) => friendKey(f.name)));
  const off = sb.friends.filter((f) => !onNames.has(friendKey(f)));
  box.innerHTML = `
    <div class="row sb-friends-head"><b>Friends</b>
      <input id="sbFriendName" placeholder="Add a friend's in-game name" autocomplete="off" spellcheck="false" maxlength="64">
      <button class="btn small" data-act="friend-add">Add</button>
      <span class="muted">${sb.friends.length ? "" : "Add the names your friends play under to see which server they're on."}</span></div>
    ${on.length ? `<div class="sb-friends-on">${on.map((f) => `<div class="sb-friend"><span class="star">★</span><b>${esc(f.name)}</b>
      <span class="muted">on ${esc(f.server.name)} · ${esc(f.server.map)} · ${f.server.players}/${f.server.capacity}</span>
      <button class="btn small primary" data-act="sb-join" data-addr="${esc(f.server.address)}">Join</button></div>`).join("")}</div>` : ""}
    ${off.length ? `<div class="sb-friends-off"><span class="muted">${sb.list?.done ? "Not playing right now:" : "Not seen yet:"}</span>
      ${off.map((n) => `<span class="sb-fchip">${esc(n)}<button data-act="friend-remove" data-name="${esc(n)}" title="Remove ${esc(n)}" aria-label="Remove ${esc(n)}">✕</button></span>`).join("")}</div>` : ""}`;
  $("#sbFriendName", box).onkeydown = (e) => { if (e.key === "Enter") act("friend-add"); };
}

function sbStatus() {
  const L = sb.list, busy = L && !L.done;
  $("#sbStatus").textContent = !L ? "" : L.error ? L.error
    : busy ? `Asking servers… ${L.answered} answered of ${L.asked}`
    : `${L.answered} of ${L.asked} servers answered · ${ago(L.updatedAt) || "just now"}`;
  $("#sbRefresh").disabled = !!busy;
}

function sbVisible() {
  const L = sb.list?.servers || [];
  return L.filter((s) => (!sb.hideEmpty || s.players > 0) && (!sb.hideFull || s.players < s.capacity) && (!sb.hidePw || !s.passwordRequired) &&
    (!sb.onlyFriends || (s.playerNames || []).some(isFriend)) &&
    (!sb.q || `${s.name} ${s.hostName} ${s.map} ${s.mode} ${s.variant} ${s.address}`.toLowerCase().includes(sb.q) || sbMatchNames(s).length));
}

// Players on s whose name matches the search.
const sbMatchNames = (s) => sb.q ? (s.playerNames || []).filter((n) => n.toLowerCase().includes(sb.q)) : [];

function sbRows() {
  const box = $("#sbRows", sb.el);
  if (!sb.list) { box.innerHTML = `<p class="muted sb-empty">Open this page to ask the servers.</p>`; return; }
  const rows = sbVisible();
  if (!rows.length) {
    box.innerHTML = `<p class="muted sb-empty">${sb.list.done ? (sb.list.servers?.length ? "No servers match." : "No servers answered.") : "Asking servers…"}</p>`;
    return;
  }
  const players = rows.reduce((n, s) => n + s.players, 0);
  box.innerHTML = rows.map((s) => {
    const full = s.players >= s.capacity, open = sb.open === s.address;
    const friends = (s.playerNames || []).filter(isFriend), found = sbMatchNames(s).filter((n) => !isFriend(n));
    const who = [...friends.map((n) => `★ ${esc(n)}`), ...found.map(esc)];
    const sub = who.length ? `<span class="who">${who.slice(0, 4).join(", ")}${who.length > 4 ? ` +${who.length - 4}` : ""}</span>`
      : s.hostName ? `<span class="muted">${esc(s.hostName)}</span>` : "";
    return `<div class="sb-row${open ? " open" : ""}" data-addr="${esc(s.address)}" title="Click for details">
      <div class="n"><b>${s.passwordRequired ? `<span class="tag" title="Needs a password to join">Password</span>` : ""}${esc(s.name)}</b>${sub}</div>
      <div>${esc(s.map)}</div>
      <div>${esc(s.mode)}</div>
      <div class="${full ? "full" : s.players ? "live" : "muted"}">${s.players} / ${s.capacity}</div>
      <div class="muted">${s.pingMs} ms</div>
      <div><button class="btn small primary" data-act="sb-join" data-addr="${esc(s.address)}"${full ? ' title="Full: the game may queue you or refuse"' : ""}>Join</button></div>
    </div>${open ? `<div class="sb-detail">
      <dl class="kv">
        <dt>Address</dt><dd>${esc(s.address)}</dd>
        <dt>Game type</dt><dd>${esc(s.variant || s.mode)}</dd>
        <dt>Version</dt><dd>${esc(s.version || "—")}${s.mods ? ` · ${s.mods} mod${s.mods > 1 ? "s" : ""}` : ""}</dd>
        <dt>Host</dt><dd>${esc(s.hostName || "—")}</dd>
        <dt>Players</dt><dd>${s.playerNames?.length ? `<div class="sb-names">${s.playerNames.map((n) => `<button class="sb-name${isFriend(n) ? " friend" : ""}" data-act="friend-toggle" data-name="${esc(n)}" title="${isFriend(n) ? "Remove from friends" : "Add to friends"}">${isFriend(n) ? "★" : "☆"} ${esc(n)}</button>`).join("")}</div>` : "None right now"}</dd>
      </dl></div>` : ""}`;
  }).join("") + `<p class="muted sb-empty">${rows.length} server${rows.length > 1 ? "s" : ""} · ${players} player${players === 1 ? "" : "s"}</p>`;
}

// Ask again when the page is opened and the list is more than a minute old.
function sbMaybeRefresh() {
  const L = sb.list;
  if (!L || (L.done && Date.now() - new Date(L.updatedAt) > 60000)) api.RefreshServerList();
}

async function sbJoin(addr) {
  const g = snap.games.find((x) => x.id === "reclaimer");
  if (!g?.installed) return toast("Install Project Reclaimer first (Play tab).", "bad");
  if (!g.requirementOk && !confirm(`${g.requirementMsg}\n\nJoin anyway?`)) return;
  const s = sb.list?.servers?.find((x) => x.address === addr);
  await call("JoinServer", addr).then(() => toast(`Joining ${s?.name ?? addr}…`, "ok")).catch(() => {});
}

function renderSettings() {
  const m = snap.mcc, L = snap.launcher;
  $("#settingsBody").innerHTML = `
    <div class="card">
      <h3>Halo: The Master Chief Collection</h3>
      <dl class="kv">
        <dt>Status</dt><dd>${m.found ? "Found" : "Not found"}${m.source === "manual" ? " (set manually)" : m.found ? " (via Steam)" : ""}</dd>
        <dt>Folder</dt><dd>${esc(m.path || "—")}</dd>
        ${m.components.map((c) => `<dt>${esc(c.name)}</dt><dd>${c.installed ? "Installed" : esc(c.detail)}</dd>`).join("")}
        <dt>Steam</dt><dd>${esc(m.steamPath || "Not found")}</dd>
        <dt>Steam libraries</dt><dd>${(m.libraries || []).map(esc).join("<br>") || "—"}</dd>
      </dl>
      <div class="row" style="margin-top:14px">
        <button class="btn small" data-act="locate-mcc">Choose folder…</button>
        ${m.source === "manual" ? `<button class="btn small" data-act="reset-mcc">Use Steam detection</button>` : ""}
        <button class="btn small" data-act="redetect">Detect again</button>
      </div>
    </div>
    <div id="gsHost"></div>
    <div class="card">
      <h3>Settings report</h3>
      <p>Export a read-only report of what's in each game's settings folder (secrets and addresses masked), to check a format the launcher doesn't know yet:</p>
      <div class="row" style="margin-top:12px"><button class="btn small" data-act="report">Export settings report</button></div>
    </div>
    <div class="card">
      <h3>Cover art</h3>
      <p>Built-in covers are included. To use your own, put a file with the name shown below in the <code>art</code> folder next to Reclaimed.exe and it replaces the built-in one; delete it to go back.</p>
      <dl class="kv">
        ${snap.games.map((g) => `<dt>${esc(g.name)}</dt><dd>Portrait: ${esc(g.artTallSrc || "none")} <span class="muted">(${esc(g.id)}-tall.png, 600×900)</span><br>Wide: ${esc(g.artWideSrc || "none")} <span class="muted">(${esc(g.id)}-wide.png, 920×430)</span></dd>`).join("")}
      </dl>
      <div class="row" style="margin-top:14px"><button class="btn small" data-act="open-art">Open art folder</button><span class="muted">.png, .jpg or .webp · picked up after Check for updates or a restart</span></div>
    </div>
    <div class="card">
      <h3>Launcher</h3>
      <dl class="kv">
        <dt>Version</dt><dd>${esc(L.version)}</dd>
        <dt>Folder</dt><dd>${esc(L.root)}</dd>
        <dt>Writes only to</dt><dd>games\\, data\\, reports\\, art\\ inside that folder; a game's own settings file when you press Save under Game settings (and once, to set borderless fullscreen); and, only when you press the buttons below, a desktop shortcut or Steam's list of non-Steam games (backed up first)</dd>
      </dl>
      <div class="row" style="margin-top:14px">
        <button class="btn small" data-act="desktop-shortcut">Create desktop shortcut</button>
        <button class="btn small" data-act="add-steam">Add to Steam</button>
        <button class="btn small" data-act="check-launcher">${L.update?.available ? `Update to ${esc(L.update.latest)}` : "Check for a new Reclaimed"}</button>
      </div>
      <p style="margin-top:12px">Not affiliated with Microsoft, Halo Studios, 343 Industries, Bungie, or either game's developers.</p>
    </div>`;
  // The game settings card lives outside this redraw so unsaved choices survive it.
  $("#gsHost").replaceWith(gsCard());
  gsRefreshRunning();
}

// ------------------------------------------------------- game settings
// One card for each game's own settings file. Edits stay in gs.edits until
// Save; the Go side checks the file is unchanged, backs it up, then writes.
const gs = { el: null, id: "reclaimer", pages: {}, edits: {}, running: {}, loading: {}, saving: false };

function gsCard() {
  if (!gs.el) {
    gs.el = document.createElement("div");
    gs.el.className = "card";
    gs.el.addEventListener("change", (e) => { if (e.target.matches("select[data-key]")) gsChange(e.target.dataset.key, e.target.value); });
    gsRender();
  }
  return gs.el;
}

async function gsLoad(id) {
  if (gs.loading[id]) return;
  gs.loading[id] = true;
  try {
    gs.pages[id] = await call("GameSettings", id).catch(() => ({ error: "Couldn't read the settings." }));
    gs.edits[id] = {};
  } finally {
    gs.loading[id] = false;
  }
  if (id === gs.id) gsRender();
}

// Redraw only when a game starts or stops, so an open dropdown isn't closed.
function gsRefreshRunning() {
  let changed = false;
  for (const g of snap.games) {
    const r = !!(g.installed && g.running);
    if (gs.running[g.id] !== r) { gs.running[g.id] = r; changed = true; }
  }
  if (changed) gsRender();
}

const gsShown = (f) => gs.edits[gs.id]?.[f.key] ?? f.value;

function gsRender() {
  if (!gs.el) return;
  const id = gs.id, p = gs.pages[id], edits = gs.edits[id] || {};
  const g = snap?.games.find((x) => x.id === id);
  const tabs = [...(snap?.games || []), { id: "sync", name: "Sync" }].map((x) => `<button class="btn small${x.id === id ? " on" : ""}" data-act="gs-tab" data-id="${esc(x.id)}">${esc(x.name)}</button>`).join("");
  let body;
  if (id === "sync") {
    body = gsSyncBody();
  } else if (!p) {
    body = `<p class="muted">Loading…</p>`;
    gsLoad(id);
  } else if (p.error) {
    body = `<p>${esc(p.error)}</p>${p.path ? `<p class="muted">File: ${esc(p.path)}</p>` : ""}
      <div class="row" style="margin-top:12px"><button class="btn small" data-act="gs-reload">Check again</button></div>`;
  } else {
    const groups = [...new Set(p.fields.map((f) => f.group))];
    const n = Object.keys(edits).length, running = gs.running[id];
    body = `${p.note ? `<p>${esc(p.note)}</p>` : ""}
      ${groups.map((grp) => `<div class="gs-group"><h4>${esc(grp)}</h4><div class="gs-grid">
        ${p.fields.filter((f) => f.group === grp).map(gsRow).join("")}
      </div></div>`).join("")}
      ${running ? `<p class="gs-warn">${esc(g?.name ?? id)} is running. Close it to change its settings.</p>` : ""}
      <div class="row" style="margin-top:16px">
        <button class="btn small primary" data-act="gs-save"${!n || running || gs.saving ? " disabled" : ""}>${gs.saving ? "Saving…" : n ? `Save ${n} change${n > 1 ? "s" : ""}` : "Save"}</button>
        <button class="btn small" data-act="gs-undo"${n ? "" : " disabled"}>Undo changes</button>
        <button class="btn small" data-act="gs-reload">Reload</button>
        <button class="btn small" data-act="gs-backups">Open backups</button>
      </div>
      <p class="muted" style="margin-top:10px">File: ${esc(p.path)} · backed up to data\\backups before every save</p>`;
  }
  gs.el.innerHTML = `<div class="row"><h3 style="flex:1">Game settings</h3><div class="row gs-tabs">${tabs}</div></div>${body}`;
}

function gsRow(f) {
  const label = `<label for="gs-${esc(f.key)}">${esc(f.label)}</label>`;
  if (f.readOnly || !f.options?.length) {
    return `${label}<div class="ro" title="${esc(f.note || "")}">${esc(f.value || "—")}${f.note ? ` <span class="muted">· ${esc(f.note)}</span>` : ""}</div>`;
  }
  const v = gsShown(f), changed = v !== f.value;
  return `${label}<select id="gs-${esc(f.key)}" data-key="${esc(f.key)}"${changed ? ' class="changed"' : ""}>
    ${f.options.map((o) => `<option value="${esc(o.value)}"${o.value === v ? " selected" : ""}>${esc(o.label)}</option>`).join("")}
  </select>`;
}

// gsSet records one field's choice (dropping it when back at the file's value).
function gsSet(key, v) {
  const p = gs.pages[gs.id], f = p.fields.find((x) => x.key === key), edits = (gs.edits[gs.id] ||= {});
  if (!f) return;
  if (v === f.value) delete edits[key]; else edits[key] = v;
}

function gsChange(key, v) {
  const p = gs.pages[gs.id];
  gsSet(key, v);
  // Presets (Reach's graphics quality) set other rows; a changed row sets the
  // preset to whichever one the rows now match, as the game's menu does.
  const presets = p.presets || {};
  if (presets[key]?.[v]) for (const [k, pv] of Object.entries(presets[key][v])) gsSet(k, pv);
  for (const [pk, table] of Object.entries(presets)) {
    if (pk === key || !Object.values(table).some((vals) => key in vals)) continue;
    const shown = (k) => gsShown(p.fields.find((x) => x.key === k));
    const match = Object.entries(table).find(([, vals]) => Object.entries(vals).every(([k, pv]) => shown(k) === pv));
    gsSet(pk, match ? match[0] : "custom");
    const pf = p.fields.find((x) => x.key === pk);
    if (pf && !match && !pf.options.some((o) => o.value === "custom")) pf.options.push({ value: "custom", label: "Custom" });
  }
  gsRender();
}

// ---- Sync: the settings both games share, side by side, copied either way.
async function gsLoadCompare() {
  if (gs.loading.sync) return;
  gs.loading.sync = true;
  try {
    gs.compare = await call("CompareSettings").catch(() => ({ error: "Couldn't read the settings." }));
  } finally {
    gs.loading.sync = false;
  }
  if (gs.id === "sync") gsRender();
}

function gsSyncBody() {
  const c = gs.compare;
  if (!c) { gsLoadCompare(); return `<p class="muted">Loading…</p>`; }
  if (c.error) return `<p>${esc(c.error)}</p><div class="row" style="margin-top:12px"><button class="btn small" data-act="gs-sync-reload">Check again</button></div>`;
  const [a, b] = (snap?.games || []).map((g) => g.id), name = (id) => snap.games.find((g) => g.id === id)?.name ?? id;
  const busy = gs.saving, blocked = (to) => busy || gs.running[to];
  const copyBtn = (r, from, to, arrow) => {
    const s = r.sides[from], off = r.same || !s.canCopy || blocked(to);
    const title = r.same ? "Already the same" : !s.canCopy ? `Can't copy: ${s.note}` : gs.running[to] ? `Close ${name(to)} first` : `Copy ${name(from)}'s ${s.label} to ${name(to)}`;
    return `<button class="btn small" data-act="gs-copy" data-from="${esc(from)}" data-to="${esc(to)}" data-key="${esc(r.key)}" title="${esc(title)}"${off ? " disabled" : ""}>${arrow}</button>`;
  };
  const copyable = (from) => c.rows.some((r) => !r.same && r.sides[from].canCopy);
  const allBtn = (from, to) => `<button class="btn small" data-act="gs-copy" data-from="${esc(from)}" data-to="${esc(to)}"${!copyable(from) || blocked(to) ? " disabled" : ""}>Copy all to ${esc(name(to))}</button>`;
  const n = c.rows.filter((r) => !r.same).length;
  const running = [a, b].filter((id) => gs.running[id]);
  return `<p>${n ? `${n} of ${c.rows.length} shared settings differ.` : `All ${c.rows.length} shared settings match.`} Copying saves straight away, with a backup first. Resolution and mouse sensitivity work differently in each game, so they aren't synced.</p>
    <div class="gs-sync">
      <div class="h"></div><div class="h">${esc(name(a))}</div><div class="h"></div><div class="h">${esc(name(b))}</div>
      ${c.rows.map((r) => `
        <div>${esc(r.label)}</div>
        <div class="${r.same ? "" : "diff"}" title="${esc(r.sides[a].note || "")}">${esc(r.sides[a].label)}</div>
        <div class="arrows">${copyBtn(r, a, b, "→")}${copyBtn(r, b, a, "←")}</div>
        <div class="${r.same ? "" : "diff"}" title="${esc(r.sides[b].note || "")}">${esc(r.sides[b].label)}</div>`).join("")}
    </div>
    ${running.map((id) => `<p class="gs-warn">${esc(name(id))} is running. Close it to copy settings to it.</p>`).join("")}
    <div class="row" style="margin-top:16px">${allBtn(a, b)}${allBtn(b, a)}<button class="btn small" data-act="gs-sync-reload">Reload</button></div>`;
}

async function gsCopy(from, to, key) {
  const name = (id) => snap.games.find((g) => g.id === id)?.name ?? id;
  if (Object.keys(gs.edits[to] || {}).length && !confirm(`You have unsaved changes on ${name(to)}'s page. Copying reloads it and discards them. Continue?`)) return;
  gs.saving = true; gsRender();
  try {
    const r = await call("CopySettings", from, to, key ? [key] : []);
    gs.pages[to] = r.page; gs.edits[to] = {};
    toast(r.backup ? `Copied to ${name(to)}. Its old settings are backed up.` : "Nothing needed copying.", "ok");
  } catch {
    delete gs.pages[to];
  } finally {
    gs.saving = false;
    gs.compare = null; // re-read both files
    gsRender();
  }
}

async function gsSave() {
  const id = gs.id, p = gs.pages[id], changes = gs.edits[id] || {};
  if (!Object.keys(changes).length) return;
  gs.saving = true; gsRender();
  try {
    const r = await call("SaveGameSettings", id, p.stamp, changes);
    gs.pages[id] = r.page; gs.edits[id] = {};
    toast(r.backup ? "Settings saved. The old file is backed up." : "Nothing needed changing.", "ok");
  } catch {
    await gsLoad(id); // the file may have changed; show what's there now
  } finally {
    gs.saving = false; gs.compare = null; gsRender();
  }
}

function renderStatus() {
  const times = snap.games.map((g) => g.checkedAt).filter(Boolean).sort();
  const last = times.length ? ago(times[times.length - 1]) : "";
  $("#updStatus").textContent = snap.checking ? "Checking for updates…" : last ? `Updates checked ${last}` : "Updates not checked yet";
  $("#checkNow").disabled = !!snap.checking;
  $("#rootPath").textContent = snap.launcher.root;
  $("#rootPath").title = snap.launcher.root;
}

function render() {
  if (!snap) return;
  renderBanners();
  renderPill();
  renderTiles();
  renderServers();
  renderSettings();
  renderStatus();
}

// ---------------------------------------------------------------- actions
async function act(name, id, el) {
  const g = snap.games.find((x) => x.id === id);
  if (name !== "menu") { openMenu = null; }
  switch (name) {
    case "menu":
      openMenu = openMenu === id ? null : id;
      renderTiles();
      return;
    case "install":
      if (g?.installed && g.running) return toast("Close the game first.", "bad");
      progress[id] = { phase: "check", done: 0, total: -1 };
      renderTiles();
      try { await call("Install", id); } catch { delete progress[id]; renderTiles(); }
      return;
    case "cancel": return api.CancelInstall(id);
    case "play":
      if (g && !g.requirementOk && !confirm(`${g.requirementMsg}\n\nTry to start ${g.name} anyway?`)) return;
      await call("Play", id).then(() => toast(`Starting ${g?.name ?? id}…`, "ok")).catch(() => {});
      return;
    case "open-art": return call("OpenArtFolder").catch(() => {});
    case "open-folder": return call("OpenGameFolder", id).catch(() => {});
    case "open-data": return call("OpenDataFolder", id).catch(() => {});
    case "release-page": return api.OpenURL(g.latestUrl);
    case "notes": return showModal(`${g.name} v${g.latest}`, md(g.latestNotes) + `<p class="muted">Released ${esc(ago(g.latestPublished))} · ${esc(g.latestAsset)}</p>`);
    case "about":
      return showModal(`${g.name} updates`, `<p>${esc(g.selfUpdateNote)}</p>
        <p>The launcher checks GitHub for a new release each time it starts. It only replaces files after verifying the download against the release's <code>SHA256SUMS.txt</code>, never while the game (or its updater) is running, and it keeps the previous version in <code>games\\.previous</code>.</p>
        <p class="muted">Game folder: ${esc(g.dir)}<br>Settings &amp; logs: ${esc(g.userDataDir)}</p>`);
    case "link": snap = await call("LinkExisting", id).catch(() => snap); return render();
    case "unlink": snap = await call("Unlink", id).catch(() => snap); return render();
    case "locate-mcc": snap = await call("ChooseMCCFolder").catch(() => snap); return render();
    case "reset-mcc": snap = await call("ResetMCCFolder"); return render();
    case "redetect": snap = await call("RedetectMCC"); toast("MCC detection refreshed."); return render();
    case "sb-refresh": return api.RefreshServerList();
    case "sb-join": return sbJoin(el.dataset.addr);
    case "sb-direct": return sbJoin($("#sbAddr").value.trim());
    case "friend-add": {
      const inp = $("#sbFriendName"), n = inp?.value.trim();
      if (!n) return inp?.focus();
      if (isFriend(n)) return toast(`${n} is already a friend.`);
      inp.value = "";
      return setFriends([...sb.friends, n]);
    }
    case "friend-remove": return setFriends(sb.friends.filter((f) => friendKey(f) !== friendKey(el.dataset.name)));
    case "friend-toggle": {
      const n = el.dataset.name;
      return setFriends(isFriend(n) ? sb.friends.filter((f) => friendKey(f) !== friendKey(n)) : [...sb.friends, n]);
    }
    case "gs-tab":
      gs.id = id;
      if (id === "sync") gs.compare = null; // always compare the files as they are now
      return gsRender();
    case "gs-sync-reload": gs.compare = null; return gsRender();
    case "gs-copy": return gsCopy(el.dataset.from, el.dataset.to, el.dataset.key);
    case "gs-save": return gsSave();
    case "gs-undo": gs.edits[gs.id] = {}; return gsLoad(gs.id);
    case "gs-reload":
      if (Object.keys(gs.edits[gs.id] || {}).length && !confirm("Discard your unsaved changes and reload?")) return;
      return gsLoad(gs.id);
    case "gs-backups": return call("OpenSettingsBackups", gs.id).catch(() => {});
    case "report": {
      const p = await call("ExportSettingsReport").catch(() => null);
      if (p) toast(`Report saved: ${p}`, "ok");
      return;
    }
    case "tip-hide": return hideTip();
    case "modal-close": $("#modal").hidden = true; return;
    case "desktop-shortcut": {
      const p = await call("CreateDesktopShortcut").catch(() => null);
      if (p) toast("Shortcut added to your desktop.", "ok");
      return;
    }
    case "add-steam": return addToSteam(false);
    case "steam-restart": $("#modal").hidden = true; return addToSteam(true);
    case "launcher-notes": {
      const U = snap.launcher.update || {};
      return showModal(`Reclaimed ${U.latest}`, md(U.notes || "No notes.") +
        `<div class="row" style="margin-top:16px"><span class="spacer"></span><button class="btn small primary" data-act="update-launcher">Update now</button></div>`);
    }
    case "check-launcher":
      if (snap.launcher.update?.available) return act("update-launcher");
      api.CheckForUpdates();
      return toast("Checking for a new Reclaimed…");
    case "update-launcher":
      $("#modal").hidden = true;
      selfUpdateText = "Downloading…";
      return call("UpdateLauncher").catch(() => {});
  }
}

async function addToSteam(restart) {
  if (restart) toast("Closing Steam for a moment…");
  const r = await call("AddToSteam", restart).catch(() => null);
  if (!r) return;
  if (r.status === "steam-running") {
    return showModal("Add Reclaimed to Steam", `<p>Steam has to close for a moment while Reclaimed is added, or Steam would undo it when it closes. Reclaimed closes Steam, adds itself, and opens Steam again.</p>
      <p class="muted">Close any game you're playing first.</p>
      <div class="row" style="margin-top:16px"><span class="spacer"></span><button class="btn small" data-act="modal-close">Cancel</button><button class="btn small primary" data-act="steam-restart">Restart Steam and add</button></div>`);
  }
  toast(r.status === "added" ? "Added to Steam, with its library artwork. Find Reclaimed in your Steam Library."
    : r.art ? "Reclaimed was already in Steam; added its library artwork. Restart Steam to see it."
    : "Reclaimed is already in your Steam library.", "ok");
}

function wire() {
  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-act]");
    if (!b) {
      if (openMenu && !e.target.closest(".menu")) { openMenu = null; renderTiles(); }
      return;
    }
    if (b.disabled) return;
    const id = b.dataset.id || b.closest("[data-id]")?.dataset.id;
    act(b.dataset.act, id, b);
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      $("#modal").hidden = true;
      if (openMenu) { openMenu = null; renderTiles(); }
    }
  });
  $("#modalClose").onclick = () => ($("#modal").hidden = true);
  $("#modal").onclick = (e) => { if (e.target.id === "modal") $("#modal").hidden = true; };
  for (const t of document.querySelectorAll(".tab")) {
    t.onclick = () => {
      document.querySelectorAll(".tab").forEach((x) => x.classList.toggle("active", x === t));
      document.querySelectorAll(".view").forEach((v) => v.classList.toggle("active", v.id === `view-${t.dataset.view}`));
    };
  }
  document.querySelectorAll("#layoutSeg button").forEach((b) => {
    b.classList.toggle("on", b.dataset.layout === layout);
    b.onclick = () => setLayout(b.dataset.layout);
  });
  new ResizeObserver(() => { fitCards(); placeMenu(); }).observe($("#tiles"));
  document.querySelector('.tab[data-view="play"]').addEventListener("click", () => requestAnimationFrame(fitCards));
  document.querySelector('.tab[data-view="servers"]').addEventListener("click", sbMaybeRefresh);
  $("#mccPill").onclick = () => document.querySelector('.tab[data-view="settings"]').click();
  $("#checkNow").onclick = () => api.CheckForUpdates();
  $("#wMin").onclick = () => rt.WindowMinimise();
  $("#wMax").onclick = () => rt.WindowToggleMaximise();
  $("#wClose").onclick = () => rt.Quit();

  rt.EventsOn("snapshot", (s) => { snap = s; render(); });
  rt.EventsOn("discovery", (d) => { if (snap) { snap.discovery = d; renderTiles(); } });
  rt.EventsOn("servers", (l) => {
    sb.list = l;
    if (sb.el) { sbRows(); sbStatus(); if (l.done) sbFriends(); }
    if (l.done && sb.friends.length) renderTiles();
  });
  rt.EventsOn("launcher-update", (p) => {
    selfUpdateText = p.total > 0 ? `${Math.floor((p.done / p.total) * 100)}%` : "Downloading…";
    renderBanners();
  });
  rt.EventsOn("progress", (p) => {
    progress[p.id] = p;
    if (p.phase === "download") {
      // Update just the bar to avoid re-rendering the tile on every chunk.
      const fill = $(`.tile[data-id="${p.id}"] .fill`);
      const txt = $(`.tile[data-id="${p.id}"] .ptext`);
      if (fill && txt && p.total > 0) {
        fill.classList.remove("indet");
        fill.style.width = `${Math.min(100, (p.done / p.total) * 100)}%`;
        txt.innerHTML = `<span>Downloading</span><span>${fmtBytes(p.done)} / ${fmtBytes(p.total)}</span>`;
        return;
      }
    }
    renderTiles();
  });
  rt.EventsOn("install-done", (r) => {
    delete progress[r.id];
    const name = snap?.games.find((g) => g.id === r.id)?.name ?? r.id;
    if (r.error === "cancelled") toast(`${name}: install cancelled.`);
    else if (r.error) toast(`${name}: ${r.error}`, "bad");
    else toast(`${name} installed and verified.`, "ok");
  });
  setInterval(() => snap && renderStatus(), 30000);
}

async function boot() {
  api = window.go?.main?.App;
  rt = window.runtime;
  if (!api || !rt) {
    document.body.innerHTML = '<p style="padding:24px">Launcher backend not available.</p>';
    return;
  }
  wire();
  snap = await api.Snapshot();
  render();
  // The friends list, and any server list the background browse already has.
  [sb.friends, sb.list] = await Promise.all([api.Friends().catch(() => []), api.ServerList().catch(() => null)]);
  sb.friends ||= [];
  if (sb.list && !sb.list.asked) sb.list = null; // no browse has started yet
  renderTiles();
  if (sb.el) { sbRows(); sbStatus(); sbFriends(); }
}

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
else boot();
