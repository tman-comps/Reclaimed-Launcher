"use strict";
// Rally UI. Talks to the Go side through Wails bindings (window.go.main.App)
// and events (window.runtime). Text from GitHub is always escaped.

const $ = (s, el = document) => el.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

let api, rt;
let snap = null;
const progress = {}; // id -> {phase, done, total}
let openMenu = null;
let layout = "cards"; // "cards" (portrait, MCC-style) or "banners" (wide header)
try { layout = localStorage.getItem("rally.layout") || layout; } catch {}

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
  setTimeout(() => el.remove(), kind === "bad" ? 9000 : 4500);
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
function renderBanners() {
  const b = [];
  const L = snap.launcher, m = snap.mcc;
  if (L.problem) b.push(`<div class="banner bad"><b>Launcher folder:</b> ${esc(L.problem)}</div>`);
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
  if (g.id === "reclaimer") chips.push(serverChip());

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

function serverChip() {
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
  try { localStorage.setItem("rally.layout", l); } catch {}
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
function renderServers() {
  const d = snap.discovery || {};
  const s = snap.servers || {};
  const ok = d.ok;
  $("#serversBody").innerHTML = `
    <div class="card">
      <div class="row"><h3 style="flex:1">Project Reclaimer</h3><button class="btn small" data-act="refresh-servers">Refresh</button></div>
      <div class="row" style="margin-top:10px;align-items:flex-end;gap:14px">
        <div class="big-num">${ok ? d.servers : "—"}</div>
        <div class="muted" style="padding-bottom:6px">${ok ? "public servers listed" : esc(d.error || "Not checked yet")}</div>
      </div>
      <dl class="kv">
        <dt>Master servers</dt><dd>${ok ? d.masters : "—"}</dd>
        <dt>Source</dt><dd>${esc(d.endpoint || "—")}</dd>
        <dt>Updated</dt><dd>${esc(ago(d.fetchedAt) || "—")} · refreshes every minute</dd>
      </dl>
      <p style="margin-top:12px">The discovery service lists signed server addresses only, so names, maps and player counts aren't shown. Pick a server in-game.</p>
    </div>
    <div class="card">
      <h3>Reach Renaissance</h3>
      <p>Servers are listed in-game under Matchmaking. There's no public list the launcher can read yet.</p>
    </div>
    <div class="card locked">
      <div class="row"><h3 style="flex:1">Combined server browser</h3><span class="tag">Later</span></div>
      <p>${esc((s.reason || "").replace(/^./, (c) => c.toUpperCase()))}.</p>
      <ul>${(s.needs || []).map((n) => `<li>${esc(n)}</li>`).join("")}</ul>
    </div>`;
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
    <div class="card locked">
      <div class="row"><h3 style="flex:1">Game settings &amp; sync</h3><span class="tag">Investigating</span></div>
      <p>Per-game settings pages (keybinds, video, audio) and syncing FOV, sensitivity, resolution and FPS cap come once each game's settings files have been checked. Nothing is ever written to them yet.</p>
      <p>Export a read-only report of what's in each game's settings folder (secrets and addresses masked) so the formats can be reviewed:</p>
      <div class="row" style="margin-top:12px"><button class="btn small" data-act="report">Export settings report</button></div>
    </div>
    <div class="card">
      <h3>Cover art</h3>
      <p>Built-in covers are included. To use your own, put a file with the name shown below in the <code>art</code> folder next to Rally.exe and it replaces the built-in one; delete it to go back.</p>
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
        <dt>Writes only to</dt><dd>games\\, data\\, reports\\, art\\ inside that folder</dd>
      </dl>
      <p style="margin-top:12px">Not affiliated with Microsoft, Halo Studios, 343 Industries, Bungie, or either game's developers.</p>
    </div>`;
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
    case "refresh-servers": snap.discovery = await call("RefreshServers").catch(() => snap.discovery); return render();
    case "report": {
      const p = await call("ExportSettingsReport").catch(() => null);
      if (p) toast(`Report saved: ${p}`, "ok");
      return;
    }
  }
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
  $("#mccPill").onclick = () => document.querySelector('.tab[data-view="settings"]').click();
  $("#checkNow").onclick = () => api.CheckForUpdates();
  $("#wMin").onclick = () => rt.WindowMinimise();
  $("#wMax").onclick = () => rt.WindowToggleMaximise();
  $("#wClose").onclick = () => rt.Quit();

  rt.EventsOn("snapshot", (s) => { snap = s; render(); });
  rt.EventsOn("discovery", (d) => { if (snap) { snap.discovery = d; renderTiles(); renderServers(); } });
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
}

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
else boot();
