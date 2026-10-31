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
    launcher: { version: "0.4.1", root: "C:\\Games\\Reclaimed" }, mcc, games, checking: false,
    discovery: scenario === "nomcc" ? { ok: false, error: "both endpoints timed out", fetchedAt: now } : { ok: true, servers: 17, masters: 2, endpoint: "http://148.251.153.44:49175/v3/discovery", fetchedAt: now },
  };
  // Server browser rows, shaped like real status replies (docs/server-browser.md).
  const servers = [
    ["Example | Big Team Battle 64 Player", "Example Host", "Valhalla", "Capture the Flag", 58, 64, 41],
    ["Vote any map | Team Slayer | FFA | Infection", "", "Guardian", "Slayer", 14, 16, 29],
    ["Custom Maps Weekend", "Sample Community", "Sandbox", "VIP", 12, 16, 44],
    ["US East | Snipers Only", "", "Narrows", "Slayer", 6, 12, 62],
    ["EU | Team Slayer 4v4", "", "The Pit", "Slayer", 0, 8, 174],
    ["Private scrims", "", "Construct", "Slayer", 3, 8, 88, true],
  ].map(([name, hostName, map, mode, players, capacity, pingMs, pw], i) => ({
    address: `10.0.0.${i + 1}:4917${i}`, name, hostName, map, mode, variant: mode, players, capacity, pingMs,
    passwordRequired: !!pw, version: "0.9.23", mods: i === 0 ? 1 : 0, playerNames: Array.from({ length: Math.min(players, 6) }, (_, j) => `Player${j + 1}`),
  }));
  let serverList = null;
  const opts = (...vs) => vs.map((v) => (Array.isArray(v) ? { value: v[0], label: v[1] } : { value: String(v), label: String(v) }));
  const fov = opts(70, 78, 85, 90, 95, 100, 110, 120);
  const onOff = opts(["on", "On"], ["off", "Off"]);
  const pct = opts(0, 25, 50, 75, 100);
  const page = (game) => game === "reclaimer"
    ? { game, path: "C:\\Users\\you\\Documents\\My Games\\Project Reclaimer\\settings.json", stamp: "x", note: "Settings not shown here are left as they are.",
        fields: [
          { key: "graphics.display_mode", label: "Window mode", group: "Video", value: "borderless", options: opts(["windowed", "Windowed"], ["borderless", "Borderless"], ["fullscreen", "Fullscreen"]) },
          { key: "graphics.vsync", label: "V-sync", group: "Video", value: "false", options: opts(["true", "On"], ["false", "Off"]) },
          { key: "graphics.frame_limit", label: "Frame limit", group: "Video", value: "144", options: opts(30, 60, 120, 144, 165, 240) },
          { key: "graphics.width", label: "Window width", group: "Video", value: "2560", readOnly: true, note: "Set by resizing the game's window." },
          { key: "field_of_view", label: "Field of view", group: "Video", value: "95", options: fov },
          { key: "vehicle_field_of_view", label: "Vehicle field of view", group: "Video", value: "100", options: fov },
          { key: "audio.master_volume", label: "Master volume", group: "Audio", value: "75", options: pct },
          { key: "audio.mute_unfocused", label: "Mute in the background", group: "Audio", value: "true", options: opts(["true", "On"], ["false", "Off"]) },
        ] }
    : { game, path: "C:\\Users\\you\\AppData\\Local\\ReachRenaissance\\settings.txt", stamp: "y",
        fields: [
          { key: "display", label: "Window mode", group: "Video", value: "fullscreen", options: opts(["windowed", "Windowed"], ["borderless", "Borderless"], ["fullscreen", "Fullscreen"]) },
          { key: "fov", label: "Field of view", group: "Video", value: "90", options: fov },
          { key: "fps", label: "Framerate limit", group: "Video", value: "144", options: opts(["0", "Unlimited"], 60, 120, 144, 240) },
          { key: "vsync", label: "V-sync", group: "Video", value: "off", options: onOff },
          { key: "gfx.preset", label: "Graphics quality", group: "Graphics", value: "enhanced", options: opts(["original", "Original"], ["enhanced", "Enhanced"], ["custom", "Custom"]) },
          { key: "gfx.shadows", label: "Shadow quality", group: "Graphics", value: "high", options: opts(["low", "Low"], ["medium", "Medium"], ["high", "High"]) },
          { key: "volume.game", label: "Game volume", group: "Audio", value: "100", options: pct },
        ] };
  const side = (value, label, canCopy = true) => ({ value, label, canCopy });
  const compareRows = [
    ["fov", "Field of view", side("95", "95"), side("90", "90")],
    ["fov.vehicle", "Vehicle / third-person field of view", side("100", "100"), side("78", "78")],
    ["fps", "Frame limit", side("144", "144"), side("144", "144")],
    ["vsync", "V-sync", side("false", "Off"), side("off", "Off")],
    ["display", "Window mode", side("borderless", "Borderless"), side("fullscreen", "Fullscreen")],
    ["volume", "Volume", side("75", "75%"), side("100", "100%")],
  ].map(([key, label, a, b]) => ({ key, label, same: a.label === b.label, sides: { reclaimer: a, "reach-renaissance": b } }));
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
    GameSettings: async (id) => page(id),
    SaveGameSettings: async (id) => ({ page: page(id), backup: "C:\\Games\\Reclaimed\\data\\backups\\x.bak" }),
    CompareSettings: async () => ({ rows: structuredClone(compareRows) }),
    CopySettings: async (from, to) => ({ page: page(to), backup: "x" }), OpenSettingsBackups: async () => {},
    ServerList: async () => serverList,
    RefreshServerList() {
      // Answers arrive over about a second, like the real browse.
      const list = { servers: [], asked: servers.length + 2, answered: 0, done: false };
      servers.forEach((s, i) => setTimeout(() => {
        list.servers = [...list.servers, s].sort((a, b) => b.players - a.players || a.pingMs - b.pingMs);
        list.answered++;
        list.done = list.answered === servers.length;
        list.updatedAt = new Date().toISOString();
        serverList = structuredClone(list);
        emit("servers", structuredClone(list));
      }, 150 * (i + 1)));
    },
    JoinServer: async (addr) => { if (!/^[A-Za-z0-9.-]{1,253}:\d{1,5}$/.test(addr)) throw "enter the server's address as host:port"; },
  };
  window.go = { main: { App: A } };
})();
