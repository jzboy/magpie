const assert = require("node:assert/strict");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { fixture } = require("./fixtures/caller-keys.cjs");
const { openPart } = require("./reader.cjs");

// 浩哥AI's About is three rows and nothing else: the version, the address
// every agent is pointed at, and the way to uninstall (the window's only:
// magpie web would take this computer's magpie off it). The upstream
// About's log, third-party notices, Report a problem and website rows went
// with the rest of it, and stay gone in every language and at the tray
// panel's width.
const words = {
  en: { version: "Version", gateway: "Gateway URL" },
  zh: { version: "版本", gateway: "网关地址" },
  "zh-TW": { version: "版本", gateway: "閘道位址" },
  ja: { version: "バージョン", gateway: "ゲートウェイ URL" },
  de: { version: "Version", gateway: "Zugangspunkt-URL" },
};
const version = "haoge-1.0";
const gateway = "http://127.0.0.1:3999";

// withSettings hands the fixture's /api/settings the version and gateway
// About names: the real settings carry both, the shared fixture doesn't.
function withSettings(base) {
  return (route) => {
    const url = new URL(route.request().url());
    if (url.pathname !== "/api/settings") return base(route);
    return base({ request: () => route.request(), fulfill: (o) => route.fulfill(o.json ? { ...o, json: { ...o.json, version, gateway } } : o) });
  };
}

async function openAbout(t, engine, lang, width, options = {}) {
  const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
  t.after(() => browser.close());
  const page = await (await browser.newContext({ viewport: { width, height: 800 }, reducedMotion: "reduce" })).newPage();
  page.setDefaultTimeout(6000);
  page.setDefaultNavigationTimeout(60000);
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route("**/*", withSettings(fixture(lang, "light", [], options)));
  await page.goto("http://magpie.test/");
  await page.locator("#prefs").click();
  await openPart(page, "about");
  await page.locator("#about .row").first().waitFor();
  return { page, errors };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(words)) {
    for (const width of [1100, 420]) {
      test(`${engine} ${lang} ${width}: About is the version, the gateway and uninstall`, async (t) => {
        const { page, errors } = await openAbout(t, engine, lang, width);
        const rows = page.locator("#about .row");
        assert.equal(await rows.count(), 3, "About's rows");
        const names = await rows.locator(".who .name").allTextContents();
        assert.equal(names[0].trim(), words[lang].version, "the first row's name");
        assert.equal(names[1].trim(), words[lang].gateway, "the second row's name");
        assert.equal(await rows.nth(0).locator("code").textContent(), version, "the version");
        assert.equal(await rows.nth(1).locator("code").textContent(), gateway, "the gateway's address");
        assert.equal(await page.locator("#uninstallRow").count(), 1, "the window has no uninstall row");
        assert.equal(await page.locator("#uninstallRow button.text.danger").count(), 1, "uninstall has no button");
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "About wider than the window");
        assert.deepEqual(errors, []);
      });
    }
  }
  test(`${engine}: magpie web has no uninstall row in About`, async (t) => {
    const { page, errors } = await openAbout(t, engine, "en", 1100, { web: true });
    const rows = page.locator("#about .row");
    assert.equal(await rows.count(), 2, "web's About rows");
    assert.equal(await page.locator("#uninstallRow").count(), 0, "web offers to uninstall the computer's magpie");
    assert.deepEqual(errors, []);
  });
}
