import { openSettings } from "./settings-helpers";
import { checkTerminalPreview } from "./terminal-preview-checks";
import { checkImagePaste } from "./image-paste-checks";
import { checkRemoteVolume } from "./remote-volume-checks";
import { checkModifierRelease } from "./modifier-release-checks";
import { checkEnter } from "./enter-checks";
import { createServer } from "vite";
import { checkSettingsPreview } from "./settings-preview-checks";
import { checkArrowRepeat, checkRawNavigation } from "./arrow-repeat-checks";
import { checkPredictionAndCursor } from "./prediction-cursor-checks";
import { chromium } from "playwright";
import { createServer as createPortProbe } from "node:net";
import { once } from "node:events";

const portProbe = createPortProbe().listen(0, "127.0.0.1");
await once(portProbe, "listening");
const port = (portProbe.address() as { port: number }).port;
await new Promise<void>((resolve, reject) => portProbe.close(error => error ? reject(error) : resolve()));
const server = await createServer({ server: { host: "127.0.0.1", port, strictPort: true }, plugins: [{ name: "keyboard-fixture", configureServer(server) {
  server.middlewares.use("/__keyboard", (_req, res, next) => { res.setHeader("Content-Type", "text/html"); void server.transformIndexHtml("/__keyboard", '<html><body><div id="root"></div><script type="module" src="/scripts/tests/keyboard-fixture.tsx"></script></body></html>').then(html => res.end(html)).catch(next); });
} }] });
await server.listen();
const address = server.httpServer!.address() as { port: number };
let browser;
try { browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH, args: ["--disable-background-networking", "--disable-component-update", "--no-first-run"] }); } catch (error) { await server.close(); throw error; }
try {
  const page = await browser.newPage({ viewport: { width: 402, height: 874 }, deviceScaleFactor: 2, hasTouch: true });
  const errors: string[] = [];
  page.on("pageerror", error => { errors.push(error.message); console.error(error.stack); });
  await page.addInitScript(() => localStorage.setItem("ottoman-trackpad-settings", JSON.stringify({ mirrorFocusedText: true, swipeTyping: false, tabKey: true, commaKey: true, periodKey: true })));
  await page.goto(`http://127.0.0.1:${address.port}/__keyboard`);
  await page.locator('[data-letter="q"]').waitFor({ state: "visible" });
  if (await page.getByRole("button", { name: "Close keyboard", exact: true }).count()) throw Error("PC remote must not offer an action to hide its always-visible keyboard");
  if (await page.getByLabel(/Trackpad\. Tap to click/).evaluate(element => element === document.activeElement)) throw Error("Remote keyboard visibility must not require trackpad focus");
  await page.getByRole("button", { name: "Open menu", exact: true }).focus();
  if (!await page.locator('[data-letter="q"]').isVisible()) throw Error("Remote keyboard must stay visible when another control has focus");
  const fixture = (fn: string) => page.evaluate(`window.fixture.${fn}`);
  const reset = async (text: string, cursor = text.length) => { await fixture(`reset(${JSON.stringify(text)}, ${cursor})`); await page.waitForTimeout(50); };
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const value = () => fixture("context.value");
  const key = (letter: string) => page.locator(`[data-letter="${letter}"]`);
  const space = page.getByRole("button", { name: "Space. Hold and drag to move the cursor", exact: true });
  assert(await page.getByRole("button", { name: "Emoji", exact: true }).count() === 0, "Letters must hide the emoji button by default");
  const symbols = page.getByRole("button", { name: "Symbols", exact: true });
  assert(await symbols.textContent() === "#+=", "The symbols page must not advertise a duplicate number keyboard");
  await symbols.click();
  await page.getByRole("button", { name: "Emoji", exact: true }).click();
  await page.getByRole("region", { name: "All emoji", exact: true }).waitFor();
  await page.getByRole("button", { name: "Letters", exact: true }).click();
  assert(await page.getByRole("button", { name: "Emoji", exact: true }).count() === 0, "Returning from emoji must restore the hidden main-page button");
  await openSettings(page);
  const emojiSetting = page.getByRole("switch", { name: "Show emoji key on letters", exact: true });
  assert(await emojiSetting.getAttribute("aria-checked") === "false", "Main-page emoji must be opt-in");
  await emojiSetting.click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  assert(await page.getByRole("button", { name: "Emoji", exact: true }).isVisible(), "Enabling the preference must restore the main-page emoji button");
  assert(await page.evaluate(() => (JSON.parse(localStorage.getItem("ottoman-trackpad-settings")!) as { emojiKey?: boolean }).emojiKey) === true, "The emoji-button preference must persist");
  await page.getByRole("button", { name: "Emoji", exact: true }).click();
  await page.getByRole("region", { name: "All emoji", exact: true }).waitFor();
  await page.getByRole("button", { name: "Letters", exact: true }).click();
  await openSettings(page);
  await emojiSetting.click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  assert(await page.getByRole("button", { name: "Emoji", exact: true }).count() === 0, "Disabling the preference must hide only the main-page emoji button");
  await checkImagePaste(page);
  await checkEnter(page);
  await checkModifierRelease(page);
  await reset("sentence", 3);
  await page.getByRole("button", { name: "End", exact: true }).click();
  await space.click(); await space.click();
  assert(await value() === "sentence. ", "Double space must work after navigation");
  await page.getByRole("button", { name: "Backspace", exact: true }).click();
  await page.getByRole("button", { name: "Backspace", exact: true }).click();
  await space.click(); await space.click();
  assert(await value() === "sentence  ", "Backspacing inserted period must suppress re-insertion");
  await reset("hel");
  await page.getByLabel("Word suggestions").getByRole("button").nth(1).waitFor();
  const suggestion = await page.getByLabel("Word suggestions").getByRole("button").nth(1).textContent();
  await page.getByLabel("Word suggestions").getByRole("button").nth(1).click();
  assert(await value() === suggestion + " ", "Suggestions must insert an immediate trailing space");
  await page.getByRole("button", { name: "Comma", exact: true }).click();
  assert(await value() === suggestion + ",", "Punctuation must remove the automatic space");
  await reset("hello\nworld\n");
  const preview = page.locator("[data-text-preview]");
  const text = preview.locator("[data-caret-text]");
  const heights: number[] = [];
  for (const lines of ["hello", "hello\n", "hello\n\n", "hello\n\n\n"]) { await reset(lines); heights.push(await text.evaluate(element => element.parentElement!.getBoundingClientRect().height)); }
  assert(heights.every((height, i) => i === 0 || Math.abs(height - heights[i - 1] - 24) < 2), `Trailing blank lines must add one line each: ${JSON.stringify(heights)}`);
  await openSettings(page);
  assert(await page.getByLabel("Remote text", { exact: true }).inputValue() === "hello\n\n\n", "Opening settings must preserve the text field");
  await page.getByRole("switch", { name: "Autocorrect", exact: true }).click();
  await page.getByRole("switch", { name: "Adaptive key sizes", exact: true }).click();
  await page.getByRole("switch", { name: "Debug key sizes", exact: true }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  await reset("keynoard"); await space.click();
  assert(await value() === "keyboard ", "Autocorrect must correct keynoard to keyboard");
  await reset("hel");
  assert(await page.locator('[data-key-weight]').evaluateAll(elements => elements.some(element => Number(element.getAttribute("data-key-weight")) > 1)), "Predictions must enlarge likely next keys");
  await checkPredictionAndCursor(page, reset);
  await checkArrowRepeat(page, reset);
  await checkRawNavigation(page);
  const shift = page.getByRole("button", { name: "Shift", exact: true });
  await reset("");
  const pointer = async (locator: ReturnType<typeof key>, type: string, id: number) => locator.evaluate((element, { type, id }) => {
    const button = element as HTMLButtonElement;
    const capture = button.setPointerCapture.bind(button);
    button.setPointerCapture = () => {};
    button.dispatchEvent(new PointerEvent(type, { bubbles: true, cancelable: true, pointerId: id, pointerType: "touch", button: 0 }));
    button.setPointerCapture = capture;
  }, { type, id });
  await pointer(shift, "pointerdown", 901);
  for (const letter of "foo") {
    await pointer(key(letter), "pointerdown", 902);
    await pointer(key(letter), "pointerup", 902);
  }
  await pointer(shift, "pointerup", 901);
  assert(await value() === "FOO", `Held Shift must apply to multiple letters: ${String(await value())}, ${JSON.stringify(await fixture("log"))}`);
  assert(await shift.getAttribute("aria-pressed") === "false", "Held Shift must release");
  await reset("abc");
  await pointer(shift, "pointerdown", 901);
  await page.getByRole("button", { name: "Arrow left", exact: true }).click();
  await page.getByRole("button", { name: "Arrow left", exact: true }).click();
  await pointer(shift, "pointerup", 901);
  assert(await fixture("context.selectionEnd - window.fixture.context.selectionStart") === 2, "Held Shift must extend a selection across multiple navigation keys");
  await reset("abcdefghij", 0);
  const spaceBox = (await space.boundingBox())!;
  const spacePoint = { x: spaceBox.x + spaceBox.width / 2, y: spaceBox.y + spaceBox.height / 2 };
  await page.mouse.move(spacePoint.x, spacePoint.y); await page.mouse.down();
  await page.waitForTimeout(380);
  await page.mouse.move(spacePoint.x + 24, spacePoint.y, { steps: 4 }); await page.mouse.up();
  const draggedPosition = await fixture("context.selectionStart") as number;
  assert(draggedPosition >= 3 && draggedPosition <= 5 && await value() === "abcdefghij", `24px spacebar motion must navigate several characters without inserting a space: ${draggedPosition}`);
  await reset("first line\nsecond line\nthird line");
  await shift.click();
  await page.getByRole("button", { name: "Arrow left", exact: true }).click();
  assert(await fixture("context.selectionEnd - window.fixture.context.selectionStart") === 1, "Shift + arrow must select one character");
  await reset(Array.from({ length: 30 }, (_, i) => `Line ${i}`).join("\n"));
  const scrollable = page.locator("[data-native-preview]");
  assert(await scrollable.evaluate(element => element.scrollHeight > element.clientHeight), "Long previews must scroll");
  await scrollable.evaluate(element => { element.scrollTop = 0; });
  assert(await scrollable.evaluate(element => element.scrollTop) === 0, "Preview scroll must allow earlier text");
  await scrollable.hover(); await page.mouse.wheel(0, 200); await page.waitForTimeout(100);
  assert(await scrollable.evaluate(element => element.scrollTop) > 0, "Wheel over the preview must scroll its text");
  assert(!(await fixture("log") as { type: string }[]).some(message => message.type === "mousescroll"), "Scrolling the preview must not scroll the remote application");
  await reset("hello\nworld", 0);
  await page.getByRole("button", { name: "Arrow down", exact: true }).click();
  assert(await fixture("context.selectionStart") === 6, "Down must follow preview line position");
  const point = await text.evaluate(element => { const range = document.createRange(); range.setStart(element.firstChild!, 2); range.setEnd(element.firstChild!, 3); const rect = range.getBoundingClientRect(); return { x: rect.x, y: rect.y + rect.height / 2 }; });
  await page.touchscreen.tap(point.x, point.y);
  assert(await fixture("context.selectionStart") === 2, "Tapping preview text must position the cursor");
  await page.screenshot({ path: "/tmp/keyboard-preview@2x.png" });
  await openSettings(page);
  await page.screenshot({ path: "/tmp/keyboard-settings@2x.png" });
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  const brightness = page.getByRole("slider", { name: "Screen brightness", exact: true });
  assert(await brightness.inputValue() === "55", "Menu brightness must show the monitor value");
  await brightness.fill("72");
  await page.waitForTimeout(50);
  assert(await fixture("brightness()") === 72, "Brightness must address the selected display");
  await page.getByRole("button", { name: "Close menu", exact: true }).last().click();
  await checkRemoteVolume(page);
  const pad = page.getByLabel(/Trackpad\. Tap to click/);
  const touchEvent = async (type: string, points: { x: number; y: number }[], changed = points) => pad.evaluate((element, { type, points, changed }) => {
    const touch = (p: { x: number; y: number }, i: number) => new Touch({ identifier: i, target: element, clientX: p.x, clientY: p.y });
    element.dispatchEvent(new TouchEvent(type, { bubbles: true, cancelable: true, touches: points.map(touch), changedTouches: changed.map(touch) }));
  }, { type, points, changed });
  const count = async (type: string) => (await fixture("log") as { type: string }[]).filter(item => item.type === type).length;
  const first = { x: 110, y: 180 }, second = { x: 160, y: 200 };
  const mouseEvents = async () => (await fixture("log") as { type: string }[])
    .filter(item => ["mousedown", "mouseup", "mouseclick", "mousemoverel"].includes(item.type)).map(item => item.type).join(",");
  for (const gap of [30, 150, 250]) {
    await reset(`drag after ${gap}ms`);
    await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
    assert(await mouseEvents() === "mousedown", "First tap feedback must press immediately while reserving the retouch window");
    await page.waitForTimeout(gap);
    await touchEvent("touchstart", [first]);
    assert(await mouseEvents() === "mousedown", "Second contact must reuse the first press instead of double-clicking");
    await page.waitForTimeout(1200);
    assert(await mouseEvents() === "mousedown", "A stationary second contact must keep the original press beyond the recognition window");
    await touchEvent("touchmove", [second]);
    assert(await mouseEvents() === "mousedown,mousemoverel", "Movement after a long hold must drag with the button still down");
    await touchEvent("touchend", [], [second]);
    assert(await mouseEvents() === "mousedown,mousemoverel,mouseup", "A drag must release as soon as the finger lifts");
    await page.waitForTimeout(350);
    assert(await count("mousedown") === 1 && await count("mouseup") === 1 && await count("mouseclick") === 0, "A drag must contain exactly one native press, with no titlebar/word double-click or delayed input");
  }
  // N physical contacts ending in a drag must produce N-1 native presses:
  // down/up/down/drag is one click-drag; down/up/down/up/down/drag is two.
  // Include six contacts to catch implementations hard-coded to triples/quads.
  for (const contacts of [3, 4, 6]) {
    for (const gap of [30, 150, 250]) {
      await reset(`${contacts}-contact drag after ${gap}ms`);
      for (let tap = 0; tap < contacts - 1; tap++) {
        await touchEvent("touchstart", [first]);
        await touchEvent("touchend", [], [first]);
        await page.waitForTimeout(gap);
      }
      await touchEvent("touchstart", [first]);
      const beforeDrag = [...Array.from({ length: contacts - 2 }, () => ["mousedown", "mouseup"]).flat(), "mousedown"].join(",");
      assert(await mouseEvents() === beforeDrag, `${contacts} contacts must hold press ${contacts - 1}, including retouches before the 40ms press timer`);
      if (gap === 150) {
        await page.waitForTimeout(700);
        assert(await mouseEvents() === beforeDrag, "A late-chain hold must not expire or add another native click");
      }
      await touchEvent("touchmove", [second]);
      assert(await mouseEvents() === `${beforeDrag},mousemoverel`, "Multi-tap dragging must move immediately with the final native press already held");
      await touchEvent("touchend", [], [second]);
      await page.waitForTimeout(350);
      assert(await mouseEvents() === `${beforeDrag},mousemoverel,mouseup`, "Multi-tap dragging must release once and leave no delayed clicks");
      assert(await count("mousedown") === contacts - 1 && await count("mouseup") === contacts - 1 && await count("mouseclick") === 0, "A chain must preserve its exact native click count");
    }
  }
  for (const contacts of [3, 4]) {
    await reset(`${contacts} stationary taps`);
    for (let tap = 0; tap < contacts; tap++) {
      await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
      if (tap < contacts - 1) await page.waitForTimeout(30);
    }
    await page.waitForTimeout(350);
    assert(await mouseEvents() === Array.from({ length: contacts }, () => ["mousedown", "mouseup"]).flat().join(","), "Stationary tap chains must complete every click without a stuck press");
  }
  for (const ending of ["cancel", "blur", "two fingers", "disconnect"]) {
    await reset(`tap chain interrupted by ${ending}`);
    for (let tap = 0; tap < 2; tap++) {
      await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
      await page.waitForTimeout(150);
    }
    await touchEvent("touchstart", [first]);
    if (ending === "blur") await page.evaluate(() => window.dispatchEvent(new Event("blur")));
    else if (ending === "disconnect") await fixture("setConnected(false)");
    else if (ending === "two fingers") { await touchEvent("touchstart", [first, second]); await touchEvent("touchcancel", [], [first, second]); }
    else await touchEvent("touchcancel", [], [first]);
    await page.waitForTimeout(350);
    assert(await mouseEvents() === "mousedown,mouseup,mousedown,mouseup", "Interrupting a tap chain must release its final press without creating another click");
    if (ending === "disconnect") { await fixture("setConnected(true)"); await pad.waitFor(); }
  }
  await reset("tap");
  await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
  assert(await mouseEvents() === "mousedown", "A single tap must give immediate press feedback");
  await page.waitForTimeout(350);
  assert(await mouseEvents() === "mousedown,mouseup", "A single tap must release once the 300ms retouch window closes");
  const jitter = { x: first.x + 2, y: first.y + 1 };
  for (const gap of [30, 150, 250]) {
    await reset(`double tap after ${gap}ms`);
    await touchEvent("touchstart", [first]); await touchEvent("touchmove", [jitter]); await touchEvent("touchend", [], [jitter]);
    await page.waitForTimeout(gap);
    await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
    assert(await mouseEvents() === "mousedown,mouseup", "Only a completed stationary second tap may request the second click");
    await page.waitForTimeout(60);
    assert(await mouseEvents() === "mousedown,mouseup,mousedown", "A real double-tap must start its second native press after a real interval, reserving it for another retouch");
    await page.waitForTimeout(300);
    assert(await mouseEvents() === "mousedown,mouseup,mousedown,mouseup", "A stationary double-tap must complete exactly two native clicks");
  }
  await reset("stationary hold");
  await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
  await touchEvent("touchstart", [first]);
  await page.waitForTimeout(1200);
  await touchEvent("touchend", [], [first]);
  assert(await mouseEvents() === "mousedown,mouseup", "A long stationary second hold must release the original press without double-clicking");
  await reset("separate taps");
  await page.waitForTimeout(350);
  await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
  await page.waitForTimeout(350);
  await touchEvent("touchstart", [first]);
  assert(await mouseEvents() === "mousedown,mouseup", "A contact outside the tap continuation window must not press on contact");
  await touchEvent("touchend", [], [first]);
  await page.waitForTimeout(350);
  assert(await mouseEvents() === "mousedown,mouseup,mousedown,mouseup", "Separate taps must finish independently");
  for (const ending of ["cancel", "blur", "two fingers"]) {
    await page.evaluate(() => window.dispatchEvent(new Event("blur")));
    await reset(`held press ended by ${ending}`);
    await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
    await touchEvent("touchstart", [first]);
    if (ending === "cancel") await touchEvent("touchcancel", [], [first]);
    else if (ending === "blur") {
      await page.evaluate(() => window.dispatchEvent(new Event("blur")));
      await touchEvent("touchend", [], [first]);
    }
    else {
      await touchEvent("touchstart", [first, second]);
      await touchEvent("touchcancel", [], [first, second]);
    }
    assert(await mouseEvents() === "mousedown,mouseup", `${ending} must release the original held press without another click`);
    await page.waitForTimeout(350);
    assert(await mouseEvents() === "mousedown,mouseup", `${ending} must cancel all delayed click/release callbacks`);
  }
  await reset("short drag");
  await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
  await page.waitForTimeout(150);
  await touchEvent("touchstart", [first]);
  await touchEvent("touchmove", [second]); await touchEvent("touchend", [], [second]);
  assert(await mouseEvents() === "mousedown,mousemoverel,mouseup", "A short drag must move immediately within one press, without waiting for a double-click timeout");
  await page.waitForTimeout(350);
  assert(await count("mousedown") === 1 && await count("mouseclick") === 0, "Short drags must never replay a delayed click");
  await reset("distant retouch");
  await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
  const distant = { x: first.x + 80, y: first.y };
  await touchEvent("touchstart", [distant]);
  assert(await mouseEvents() === "mousedown,mouseup", "Distant retouch must release the original click without arming a drag");
  await touchEvent("touchmove", [{ x: distant.x + 20, y: distant.y }]);
  assert(await count("mousedown") === 1, "Movement after a distant retouch must not press a second button");
  await touchEvent("touchcancel", [], [distant]);
  for (const ending of ["blur", "cancel", "two fingers"]) {
    await reset(`double click interrupted by ${ending}`);
    await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
    await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
    if (ending === "blur") await page.evaluate(() => window.dispatchEvent(new Event("blur")));
    else if (ending === "cancel") await touchEvent("touchcancel", [], [first]);
    else { await touchEvent("touchstart", [first, second]); await touchEvent("touchcancel", [], [first, second]); }
    await page.waitForTimeout(80);
    assert(await mouseEvents() === "mousedown,mouseup", `${ending} must suppress a pending next press`);
  }
  await reset("ordinary cursor movement");
  await touchEvent("touchstart", [first]); await touchEvent("touchmove", [second]); await touchEvent("touchend", [], [second]);
  await touchEvent("touchstart", [second]);
  assert(await count("mousedown") === 0, "Cursor movement followed by another contact must not arm a drag");
  await touchEvent("touchcancel", [], [second]);
  await reset("scroll");
  await touchEvent("touchstart", [first, second]);
  await touchEvent("touchmove", [{ x: 110, y: 185 }, { x: 160, y: 205 }]);
  assert(await count("mousescroll") === 0, "Initial finger placement must not jump the scroll position");
  await page.waitForTimeout(25);
  await touchEvent("touchmove", [{ x: 110, y: 195 }, { x: 160, y: 215 }]);
  assert(await count("mousescroll") === 1, "Scrolling must start from the settled midpoint");
  await touchEvent("touchcancel", [], [first, second]);
  await reset("preview after settings");
  await openSettings(page);
  await page.screenshot({ path: "/tmp/keyboard-settings@2x.png" });
  await page.getByRole("switch", { name: "Tab beside space", exact: true }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  assert(await page.getByRole("button", { name: "Tab", exact: true }).count() === 0, "Tab must disappear from the main keyboard when disabled");
  const shiftBox = (await page.getByRole("button", { name: "Shift", exact: true }).boundingBox())!;
  await page.getByRole("button", { name: "Symbols", exact: true }).click();
  const relocatedTab = page.getByRole("button", { name: "Tab", exact: true });
  const tabBox = (await relocatedTab.boundingBox())!;
  assert(Math.abs(tabBox.x - shiftBox.x) < 1 && Math.abs(tabBox.y - shiftBox.y) < 1, "The relocated Tab must occupy Shift's position on #+=");
  await relocatedTab.click();
  assert((await fixture("log") as { type: string; key?: string }[]).some(message => message.type === "keydown" && message.key === "Tab"), "Relocated Tab must send Tab");
  await page.getByRole("button", { name: "Letters", exact: true }).click();
  await reset("preview after settings");
  const dotBox = (await page.locator("[data-native-text-status]").boundingBox())!;
  const dismissBox = (await page.getByRole("button", { name: "Dismiss preview", exact: true }).boundingBox())!;
  assert(dotBox.x + dotBox.width <= dismissBox.x && Math.abs(dotBox.y - dismissBox.y) < 2, "Preview status must sit directly left of the dismiss button");
  await openSettings(page);
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  await reset("dismiss keeps this text");
  await page.getByRole("button", { name: "Dismiss preview", exact: true }).click();
  assert(!await preview.isVisible() && await value() === "dismiss keeps this text", "Dismissing must hide the preview without clearing the remote field");
  assert((await fixture("log") as unknown[]).length === 0, "Dismissing must send no desktop input");
  await key("q").click();
  assert(await value() === "dismiss keeps this textq" && await preview.isVisible(), "Typing must restore the dismissed preview at the same insertion point");
  for (const gesture of ["pending tap", "stationary retouch", "active drag", "pending next press"]) {
    await reset(`disconnect during ${gesture}`);
    await touchEvent("touchstart", [first]); await touchEvent("touchend", [], [first]);
    if (gesture !== "pending tap") await touchEvent("touchstart", [first]);
    if (gesture === "active drag") await touchEvent("touchmove", [second]);
    if (gesture === "pending next press") await touchEvent("touchend", [], [first]);
    await fixture("setConnected(false)");
    await page.waitForTimeout(350);
    assert(await count("mousedown") === 1 && await count("mouseup") === 1 && await count("mouseclick") === 0, `Disconnect during ${gesture} must release once and leave no delayed click`);
    await fixture("setConnected(true)");
    await pad.waitFor();
  }
  await checkSettingsPreview(page, reset);
  await checkTerminalPreview(page);
  assert(errors.length === 0, `Browser errors: ${errors.join("; ")}`);
  console.log("PASS: navigation, period undo, suggestion spacing, blank lines, settings preview, autocorrect, adaptive keys, held Shift, selection, scrolling, brightness, emoji-key preference, single/double taps and arbitrary tap-chain dragging");
} finally { await browser.close(); await server.close(); }
