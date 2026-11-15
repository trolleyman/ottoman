import { createServer } from "vite";
import { chromium } from "playwright";
import { createServer as probe } from "node:net";
import { once } from "node:events";
import { mkdir } from "node:fs/promises";
import { openMenu, closeMenu, openSettings } from "./settings-helpers";
const listener = probe().listen(0, "127.0.0.1"); await once(listener, "listening");
const port = (listener.address() as { port: number }).port;
await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
const server = await createServer({ server: { host: "127.0.0.1", port, strictPort: true }, plugins: [{ name: "remote-fixture", configureServer(server) {
  server.middlewares.use("/__remote", (_req, res, next) => { res.setHeader("Content-Type", "text/html"); void server.transformIndexHtml("/__remote", '<html><body><div id="root"></div><script type="module" src="/scripts/tests/keyboard-fixture.tsx"></script></body></html>').then(html => res.end(html)).catch(next); });
} }] });
await server.listen();
const browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH });
const out = process.argv[2] || "/tmp/ottoman-remote-implemented"; await mkdir(out, { recursive: true });
try {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, hasTouch: true });
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.stack || error.message));
  const assert = (value: unknown, message: string) => { if (!value) throw Error(message); };
  const fixture = (code: string) => page.evaluate(`window.fixture.${code}`);
  await page.addInitScript(() => {
    Object.defineProperty(document, "fullscreenEnabled", { configurable: true, value: false });
    localStorage.setItem("ottoman-trackpad-settings", JSON.stringify({ mirrorFocusedText: true, swipeTyping: false }));
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { readText: () => Promise.resolve("From phone 👋"), writeText: (value: string) => { (window as unknown as { copiedText: string }).copiedText = value; return Promise.resolve(); } } });
  });
  let clipboard = "From PC £"; const mediaActions: string[] = [];
  await page.route("**/api/clipboard/text", async route => {
    if (route.request().method() === "POST") { clipboard = route.request().postData() || ""; await route.fulfill({ status: 204 }); }
    else await route.fulfill({ status: 200, body: clipboard, headers: { "Content-Type": "text/plain; charset=utf-8" } });
  });
  await page.route("**/api/media", async route => {
    if (route.request().method() === "POST") { mediaActions.push(route.request().postData() || ""); await route.fulfill({ status: 204 }); }
    else await route.fulfill({ json: { available: true, player: "org.mpris.MediaPlayer2.fixture", title: "Something About Us", artist: "Daft Punk", playing: true, canToggle: true, canPrevious: true, canNext: false } });
  });
  await page.goto(`http://127.0.0.1:${port}/__remote`);
  await page.locator('[data-letter="q"]').waitFor();
  assert(await page.getByRole("button", { name: "Enter fullscreen", exact: true }).count() === 0, "Unsupported fullscreen must have no button");
  assert(await page.locator('.custom-keyboard-shell [aria-label="Keyboard settings"]').count() === 0, "Settings must be outside the keyboard");
  assert(await page.getByRole("button", { name: "Tab", exact: true }).count() === 0 && await page.getByRole("button", { name: "Comma", exact: true }).count() === 0 && await page.getByRole("button", { name: "Period", exact: true }).count() === 0, "Optional keys must default off");
  const field = page.getByLabel("Remote text", { exact: true }); const pad = page.getByLabel(/Trackpad\. Tap to click/);
  await field.fill("Select and edit this text normally.");
  assert(await fixture("context.value") === "Select and edit this text normally.", "Native edits must reach the desktop text model");
  const before = (await pad.boundingBox())!; await page.getByRole("button", { name: "Dismiss preview" }).click();
  assert(JSON.stringify(await pad.boundingBox()) === JSON.stringify(before), "Preview dismissal must not resize the trackpad");
  await page.locator('[data-letter="q"]').click();
  assert(await field.inputValue() === "Select and edit this text normally.q", "Typing must restore the same field and insertion position");
  assert((await field.boundingBox())!.height === 64, "Text field must stay two lines tall");
  await page.screenshot({ path: `${out}/phone@2x.png` });
  await fixture("addTV()"); await openMenu(page);
  await page.getByRole("button", { name: "Pause", exact: true }).waitFor();
  assert(await page.getByRole("button", { name: "Next track", exact: true }).isDisabled(), "Unsupported media actions must be disabled");
  await page.getByRole("button", { name: "Pause", exact: true }).click(); assert(mediaActions.length === 1, "Playback must target the desktop player only on request");
  await page.getByLabel("Display layout", { exact: true }).selectOption("desk");
  assert(await fixture("context.value") === "Select and edit this text normally.q", "Layout selection must preserve the text field");
  await page.getByRole("button", { name: "TV power", exact: true }).click();
  await page.waitForFunction(() => document.querySelector('button[aria-label="TV power"]')?.getAttribute("aria-busy") === "false");
  assert((await fixture("powerWrites") as unknown[]).length === 1, "TV power must use the confirmed desktop control");
  await page.screenshot({ path: `${out}/menu@2x.png` }); await closeMenu(page);
  await page.getByRole("button", { name: "Clipboard", exact: true }).click();
  await page.getByRole("button", { name: "Copy from PC", exact: true }).click();
  await page.getByRole("dialog", { name: "PC clipboard" }).waitFor();
  assert(await page.getByLabel("Clipboard text").inputValue() === "From PC £", "Copy must load PC clipboard text into a preview");
  await page.getByRole("button", { name: "Copy to phone", exact: true }).click();
  assert(await page.evaluate(() => (window as unknown as { copiedText: string }).copiedText) === "From PC £", "Copy to phone must happen on an explicit tap");
  await page.getByRole("button", { name: "Close clipboard", exact: true }).click();
  await page.getByRole("button", { name: "Paste from phone", exact: true }).click();
  await page.waitForFunction(() => (document.querySelector('textarea[aria-label="Clipboard text"]') as HTMLTextAreaElement)?.value === "From phone 👋");
  await page.getByRole("button", { name: "Paste on PC", exact: true }).click();
  await page.getByRole("dialog", { name: "Paste from phone" }).waitFor({ state: "hidden" });
  assert(clipboard === "From phone 👋", "Paste must transfer text before sending the paste shortcut");
  await page.getByRole("button", { name: "Clipboard", exact: true }).click(); await page.getByRole("button", { name: "Function keys", exact: true }).click();
  await openSettings(page); await page.screenshot({ path: `${out}/settings@2x.png` });
  await page.getByRole("tab", { name: "Trackpad", exact: true }).click();
  await page.getByLabel("Hold duration", { exact: true }).fill("300"); await page.getByLabel("Movement tolerance", { exact: true }).fill("8");
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  await fixture('reset("hold")');
  const touch = async (type: string, points: { x: number; y: number }[], changed = points) => pad.evaluate((el, { type, points, changed }) => {
    const make = (p: { x: number; y: number }, index: number) => new Touch({ identifier: index, target: el, clientX: p.x, clientY: p.y });
    el.dispatchEvent(new TouchEvent(type, { bubbles: true, cancelable: true, touches: points.map(make), changedTouches: changed.map(make) }));
  }, { type, points, changed });
  const point = { x: 110, y: 180 };
  await touch("touchstart", [point]); await page.waitForTimeout(650); await touch("touchend", [], [point]);
  const right = await fixture('log.filter(item => item.type === "mouseclick")') as { btn: string }[];
  assert(right.length === 1 && right[0].btn === "right", "Stationary hold must right-click exactly once and suppress left-click release");
  await fixture('reset("moving")'); await touch("touchstart", [point]); await touch("touchmove", [{ x: 140, y: 180 }]); await page.waitForTimeout(400); await touch("touchend", [], [{ x: 140, y: 180 }]);
  assert((await fixture('log.filter(item => item.type === "mouseclick")') as unknown[]).length === 0, "Movement must cancel hold-to-right-click");
  await fixture('reset("menu cancels hold")');
  await touch("touchstart", [point]); await openMenu(page); await page.waitForTimeout(400); await closeMenu(page);
  assert((await fixture('log.filter(item => item.type === "mouseclick")') as unknown[]).length === 0, "Opening the menu must cancel a pending right-click");
  await fixture('reset("menu cancels reserved press")');
  await touch("touchstart", [point]); await touch("touchend", [], [point]);
  await openMenu(page); await page.waitForTimeout(350); await closeMenu(page);
  assert((await fixture('log.filter(item => item.type === "mousedown" || item.type === "mouseup")') as unknown[]).length === 2, "Opening the menu must release a pending tap once");
  for (const width of [320, 375, 768, 1280]) {
    await page.setViewportSize({ width, height: 850 });
    await page.getByRole("button", { name: "Symbols", exact: true }).click(); await page.getByRole("button", { name: "Emoji", exact: true }).click();
    await page.getByRole("region", { name: "All emoji", exact: true }).waitFor();
    assert(!await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), `Emoji must fit ${width}px without horizontal overflow`);
    await page.getByRole("button", { name: "Letters", exact: true }).click();
    if (width === 1280) await page.screenshot({ path: `${out}/desktop@2x.png` });
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await fixture('setField({password:true,available:false,editable:false,value:"private value must not appear",selectionStart:0,selectionEnd:0})');
  await page.locator('[data-password-preview]').waitFor();
  await page.locator('[data-letter="q"]').click();
  assert(await page.locator('[data-native-preview]').count() === 0, "Password input must never render in the native preview");
  const masked = await page.locator('[data-password-preview] [data-caret-text]').textContent();
  assert(masked?.replace(/[•\u200b]/g, '') === '', "Password preview must contain only a mask");
  await fixture('setField({protected:true,available:false,editable:false,value:"protected value must not appear"})');
  assert(await page.locator('[data-native-preview]').count() === 0, "Protected fields must never expose a native preview");
  await fixture('setField({value:"Restored",selectionStart:8,selectionEnd:8})');
  await fixture("setConnected(false)");
  await page.locator(".custom-keyboard-shell[inert]").waitFor();
  assert(await page.locator(".custom-keyboard-shell").getAttribute("inert") !== null && await pad.isVisible(), "Offline must preserve an inert keyboard and trackpad");
  await page.screenshot({ path: `${out}/disconnected@2x.png` });
  assert(!errors.length, errors.join("\n"));
  console.log("PASS: responsive shell, native editing, fixed field, media capabilities, bidirectional clipboard, configurable right-click, emoji widths and disconnected remote");
  console.log(out);
} finally { await browser.close(); await server.close(); }
