// A routing group's requested-model provider selector lets one named member
// keep its scheduling identity while receiving group/<id>@<model> requests.
// The API fixture is local and contains no credentials.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(process.env.MAGPIE_ROUTING_ASSETS || path.join(__dirname, "../assets"));
const models = [
  { id: "oa/gpt-6.1-sol", name: "gpt-6.1-sol", providerName: "OpenAI", icon: "generic" },
  { id: "rl/grok-4.7", name: "grok-4.7", providerName: "Grok", icon: "generic" },
];
const base = {
  id: "family", name: "Family", members: ["oa/gpt-6.1-sol", "rl/grok-4.7"],
  requestModelProviders: ["oa"], routing: "order", ready: true,
  memberInfo: [{ id: "oa/gpt-6.1-sol", ready: true }, { id: "rl/grok-4.7", ready: true }],
};

function serve(lang, posts) {
  const state = { agents: [{ id: "codex", name: "Codex", path: "/test/codex", fields: [] }], profiles: [], settings: { lang, theme: "light" } };
  let group = structuredClone(base);
  return async (r) => {
    const url = new URL(r.request().url());
    const json = (data) => r.fulfill({ json: data });
    if (url.pathname === "/boot.js") return r.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return r.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) return new Promise(() => {});
      return json({ mine: true, now: new Date().toISOString(), seq: 1, totals: { requests: 0, rerouted: 0, errors: 0 }, routes: [] });
    }
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/groups") return json({ models, pools: [], groups: [group], gatewayKeys: [], accounts: {} });
    if (url.pathname === "/api/groups/save") {
      const body = JSON.parse(r.request().postData() || "{}");
      posts.push(body);
      group = { ...group, ...body, memberInfo: group.memberInfo };
      return json({ models, pools: [], groups: [group], gatewayKeys: [], accounts: {} });
    }
    if (url.pathname.startsWith("/api/groups/")) return json({ models, pools: [], groups: [group], gatewayKeys: [], accounts: {} });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3999" } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await r.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: { label: "Requested model providers", hint: "These providers receive the model named after the group; their selected member keeps its routing settings.", none: "None" },
  zh: { label: "请求模型供应商", hint: "请求在组名后指定模型时，由这些供应商接收；所选成员仍保留原有路由设置。", none: "无" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const [lang, w] of Object.entries(words)) {
    for (const width of [1100, 420]) {
      test(`${engine} ${lang} ${width}: requested-model providers are editable`, async (t) => {
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        t.after(() => browser.close());
        const page = await (await browser.newContext({ viewport: { width, height: 1400 }, reducedMotion: "reduce" })).newPage();
        page.setDefaultTimeout(6000);
        page.setDefaultNavigationTimeout(60000);
        const posts = [], errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang, posts));
        await page.goto("http://magpie.test/?view=groups");
        await page.locator('.rt-group[data-id="family"]').waitFor();
        await page.locator('.rt-group[data-id="family"]').click();
        const ed = page.locator(".rt-gedit");
        await ed.waitFor();
        const label = ed.locator("label.rt-grequest-label");
        assert.equal(await label.textContent(), w.label);
        const hint = ed.locator(".rt-grequest .hint");
        assert.equal(await hint.textContent(), w.hint);
        const pick = ed.locator("button.rt-greq-pick");
        assert.equal(await pick.textContent(), "OpenAI");
        await pick.click();
        const menu = page.locator(".proto-menu.rt-greq-menu");
        await menu.waitFor();
        assert.equal(await menu.locator(".pm-item").count(), 2);
        assert.equal(await menu.locator('.pm-item[aria-checked="true"] .pm-name').textContent(), "OpenAI");
        await menu.locator('.pm-item[aria-checked="false"]', { hasText: "Grok" }).click();
        await page.keyboard.press("Escape");
        assert.match(await pick.textContent(), /2/);
        await ed.locator("button.primary").click();
        await page.waitForFunction(() => !document.querySelector(".rt-gedit"));
        assert.deepEqual(posts.at(-1).requestModelProviders, ["oa", "rl"]);

        await page.locator('.rt-group[data-id="family"]').click();
        await ed.waitFor();
        assert.match(await ed.locator("button.rt-greq-pick").textContent(), /2/);
        await ed.locator("button.rt-greq-pick").click();
        const menu2 = page.locator(".proto-menu.rt-greq-menu");
        assert.equal(await menu2.locator('.pm-item[aria-checked="true"]').count(), 2);
        await menu2.locator('.pm-item[aria-checked="true"]', { hasText: "OpenAI" }).click();
        await menu2.locator('.pm-item[aria-checked="true"]', { hasText: "Grok" }).click();
        await page.keyboard.press("Escape");
        assert.equal(await ed.locator("button.rt-greq-pick").textContent(), w.none);
        await ed.locator("button.primary").click();
        await page.waitForFunction(() => !document.querySelector(".rt-gedit"));
        assert.deepEqual(posts.at(-1).requestModelProviders, []);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "no sideways scroll");
        assert.deepEqual(errors, []);
      });
    }
  }
}
