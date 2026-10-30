// Renders the UI with the mock backend and saves screenshots.
// Usage: node frontend/dev/preview.js <outdir>
const { chromium } = require("playwright");
const path = require("path");
const fs = require("fs");
const out = process.argv[2] || ".";
const artDir = process.argv[3] || ""; // optional folder with <id>-tall/-wide images
const dist = path.join(__dirname, "..", "dist", "index.html");
const mock = fs.readFileSync(path.join(__dirname, "mock.js"), "utf8");
(async () => {
  const browser = await chromium.launch();
  const shots = [
    ["art-cards", "ready", null, { art: true, layout: "cards" }],
    ["art-banners", "ready", null, { art: true, layout: "banners" }],
    ["art-cards-max", "ready", null, { art: true, layout: "cards", w: 1920, h: 1040 }],
    ["art-banners-max", "missing", null, { art: true, layout: "banners", w: 1920, h: 1040 }],
    ["noart-cards", "ready", null, { layout: "cards" }],
    ["noart-banners", "ready", null, { layout: "banners" }],
    ["art-cards-min", "installing", async (p) => { await p.click('.tile[data-id="reach-renaissance"] [data-act=menu]'); }, { art: true, layout: "cards", w: 960, h: 620 }],
    ["play", "ready", null],
    ["play-missing", "missing", null],
    ["play-nomcc", "nomcc", null],
    ["installing", "installing", async (p) => { await p.click('.tile[data-id="reach-renaissance"] [data-act=install]'); await p.evaluate(() => window.runtime); }],
    ["menu", "ready", async (p) => { await p.click('.tile[data-id="reclaimer"] [data-act=menu]'); }],
    ["notes", "ready", async (p) => { await p.click('.tile[data-id="reclaimer"] [data-act=menu]'); await p.click('.menu [data-act=notes]'); }],
    ["servers", "ready", async (p) => { await p.click('.tab[data-view=servers]'); }],
    ["settings", "ready", async (p) => { await p.click('.tab[data-view=settings]'); }],
  ];
  const errors = [];
  for (const [name, s, fn, o = {}] of shots) {
    if (o.art && !artDir) continue;
    const page = await browser.newPage({ viewport: { width: o.w || 1180, height: o.h || 740 } });
    await page.addInitScript(`try { localStorage.setItem("reclaimed.layout", ${JSON.stringify(o.layout || "banners")}) } catch {}`);
    page.on("pageerror", (e) => errors.push(`${name}: ${e.message}`));
    page.on("console", (m) => m.type() === "error" && errors.push(`${name}: ${m.text()}`));
    await page.addInitScript({ content: mock });
    await page.goto("file://" + dist + "?s=" + s + (o.art ? "&art=" + encodeURIComponent("file://" + path.resolve(artDir)) : ""));
    await page.waitForSelector(".tile");
    if (fn) await fn(page);
    await page.waitForTimeout(250);
    await page.screenshot({ path: path.join(out, name + ".png") });
    await page.close();
  }
  await browser.close();
  if (errors.length) { console.error(errors.join("\n")); process.exit(1); }
  console.log("ok");
})();
