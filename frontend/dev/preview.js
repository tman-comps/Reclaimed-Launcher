// Renders the UI with the mock backend and saves screenshots.
// Usage: node frontend/dev/preview.js <outdir>
const { chromium } = require("playwright");
const path = require("path");
const fs = require("fs");
const out = process.argv[2] || ".";
const dist = path.join(__dirname, "..", "dist", "index.html");
const mock = fs.readFileSync(path.join(__dirname, "mock.js"), "utf8");
(async () => {
  const browser = await chromium.launch();
  const shots = [
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
  for (const [name, s, fn] of shots) {
    const page = await browser.newPage({ viewport: { width: 1180, height: 740 } });
    page.on("pageerror", (e) => errors.push(`${name}: ${e.message}`));
    page.on("console", (m) => m.type() === "error" && errors.push(`${name}: ${m.text()}`));
    await page.addInitScript({ content: mock });
    await page.goto("file://" + dist + "?s=" + s);
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
