// Dev-only stand-in for the Wails backend so the UI can be previewed in a
// normal browser. Not embedded in the exe (only frontend/dist is).
(() => {
  const scenario = new URLSearchParams(location.search).get("s") || "ready";
  const now = new Date().toISOString();
  const handlers = {};
  const games = [
    {
      id: "reclaimer", name: "Project Reclaimer", tagline: "Halo 3 multiplayer client", needsName: "Halo 3",
      installed: true, linked: false, dir: "C:\\Games\\Reclaimed\\games\\reclaimer", exe: "", version: "1.4.0", versionSource: "launcher",
      updatedInGame: false, latest: "1.5.0", latestUrl: "https://example.invalid", latestNotes: "## Changes\n- Faster map loads\n- **Fixed** a crash when leaving a lobby\n- New `--windowed` option",
      latestPublished: now, latestAsset: "project-reclaimer-client-v1.5.0.exe", latestSize: 48234496, checkedAt: now,
      updateAvailable: true, running: false, requirementOk: true, userDataDir: "C:\\Users\\you\\Documents\\My Games\\Project Reclaimer", userDataExists: true,
      selfUpdateNote: "Updates itself from its in-game menu by replacing its exe. The launcher recognises those updates by checksum.",
    },
    {
      id: "reach-renaissance", name: "Reach Renaissance", tagline: "Halo: Reach multiplayer, Firefight & Campaign", needsName: "Halo: Reach",
      installed: false, linked: false, dir: "C:\\Games\\Reclaimed\\games\\reach-renaissance", version: "", versionSource: "", latest: "0.1.4",
      latestUrl: "https://example.invalid", latestNotes: "First public test build.", latestPublished: now, latestAsset: "ReachRenaissance-v0.1.4.zip",
      latestSize: 312475648, checkedAt: now, updateAvailable: false, running: false, requirementOk: true,
      userDataDir: "C:\\Users\\you\\AppData\\Local\\ReachRenaissance", userDataExists: false,
      selfUpdateNote: "Offers signed updates when it starts (Update and restart). The launcher steps aside while it runs.",
    },
  ];
  const mcc = {
    steamFound: true, steamPath: "C:\\Program Files (x86)\\Steam", libraries: ["C:\\Program Files (x86)\\Steam", "D:\\SteamLibrary"],
    found: true, path: "D:\\SteamLibrary\\steamapps\\common\\Halo The Master Chief Collection", source: "steam",
    components: [{ id: "halo3", name: "Halo 3", installed: true, detail: "" }, { id: "haloreach", name: "Halo: Reach", installed: true, detail: "" }],
  };
  if (scenario === "missing") {
    mcc.components[1].installed = false;
    mcc.components[1].detail = "not installed in MCC; install it in Steam (Manage > DLC)";
    games[1].requirementOk = false;
    games[1].requirementMsg = "Halo: Reach isn't installed in MCC. Install it in Steam: MCC > Manage > DLC.";
    games[0].running = true; games[0].updateAvailable = false; games[0].version = "1.5.0"; games[0].versionSource = "release";
  }
  if (scenario === "nomcc") {
    Object.assign(mcc, { found: false, path: "", components: mcc.components.map((c) => ({ ...c, installed: false, detail: "MCC not found" })) });
    games.forEach((g) => { g.requirementOk = false; g.requirementMsg = "Halo: The Master Chief Collection (Steam) wasn't found."; });
  }
  const art = new URLSearchParams(location.search).get("art");
  if (art) {
    Object.assign(games[0], { artTall: `${art}/reclaimer-tall.webp`, artWide: `${art}/reclaimer-wide.webp` });
    Object.assign(games[1], { artTall: `${art}/reach-renaissance-tall.png`, artWide: `${art}/reach-renaissance-wide.png` });
  }
  const snap = {
    launcher: { version: "0.3.0", root: "C:\\Games\\Reclaimed" }, mcc, games, checking: false,
    discovery: scenario === "nomcc" ? { ok: false, error: "both endpoints timed out", fetchedAt: now } : { ok: true, servers: 17, masters: 2, endpoint: "http://148.251.153.44:49175/v3/discovery", fetchedAt: now },
    servers: { available: false, reason: "not available yet: waiting for the game's developers to document a server query and join method",
      needs: ["Project Reclaimer: a documented per-server info query (the discovery list has addresses only) and a join argument or URI", "Reach Renaissance: a public server list API and a join argument or URI"] },
  };
  const emit = (n, d) => (handlers[n] || []).forEach((f) => f(d));
  window.runtime = {
    EventsOn: (n, f) => { (handlers[n] ||= []).push(f); },
    WindowMinimise() {}, WindowToggleMaximise() {}, Quit() {},
  };
  const A = {
    Snapshot: async () => structuredClone(snap),
    Install: async (id) => {
      let done = 0; const total = snap.games.find((g) => g.id === id).latestSize;
      if (scenario === "installing") return; // freeze mid-download for screenshots
      const t = setInterval(() => {
        done += total / 12;
        emit("progress", { id, phase: "download", done: Math.min(done, total), total });
        if (done >= total) { clearInterval(t); emit("install-done", { id, error: "" }); }
      }, 120);
    },
    CancelInstall() {}, Play: async () => {}, OpenGameFolder: async () => {}, OpenDataFolder: async () => {}, OpenURL() {},
    CheckForUpdates() {}, RefreshServers: async () => snap.discovery, RedetectMCC: async () => structuredClone(snap),
    ChooseMCCFolder: async () => structuredClone(snap), ResetMCCFolder: async () => structuredClone(snap),
    LinkExisting: async () => structuredClone(snap), Unlink: async () => structuredClone(snap),
    OpenArtFolder: async () => {}, ExportSettingsReport: async () => "C:\\Games\\Reclaimed\\reports\\settings-report.txt",
  };
  window.go = { main: { App: A } };
})();
