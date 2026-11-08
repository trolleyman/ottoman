import { createServer } from "vite";
import { chromium } from "playwright";

const server = await createServer({ server: { host: "127.0.0.1", port: 0 }, plugins: [{ name: "keyboard-fixture", configureServer(server) {
  server.middlewares.use("/__keyboard", (_req, res, next) => { res.setHeader("Content-Type", "text/html"); void server.transformIndexHtml("/__keyboard", '<html><body><div id="root"></div><script type="module" src="/scripts/tests/keyboard-fixture.tsx"></script></body></html>').then(html => res.end(html)).catch(next); });
} }] });
await server.listen();
const address = server.httpServer!.address() as { port: number };
let browser;
try { browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH }); } catch (error) { await server.close(); throw error; }
try {
  const page = await browser.newPage({ viewport: { width: 402, height: 874 }, deviceScaleFactor: 2, hasTouch: true });
  const errors: string[] = [];
  page.on("pageerror", error => { errors.push(error.message); console.error(error.message); });
  await page.addInitScript(() => localStorage.setItem("ottoman-trackpad-settings", JSON.stringify({ mirrorFocusedText: true, swipeTyping: false })));
  await page.goto(`http://127.0.0.1:${address.port}/__keyboard`);
  await page.getByLabel("Trackpad. Tap to click, move one finger to point, tap two fingers to right click, or move two fingers to scroll.").focus();
  const fixture = (fn: string) => page.evaluate(`window.fixture.${fn}`);
  const reset = async (text: string, cursor = text.length) => { await fixture(`reset(${JSON.stringify(text)}, ${cursor})`); await page.waitForTimeout(50); };
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const value = () => fixture("context.value");
  const key = (letter: string) => page.locator(`[data-letter="${letter}"]`);
  const space = page.getByRole("button", { name: "Space. Hold and drag to move the cursor", exact: true });
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
  assert(heights.every((height, i) => i === 0 || Math.abs(height - heights[i - 1] - 20) < 2), `Trailing blank lines must add one line each: ${JSON.stringify(heights)}`);
  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  assert(await preview.isVisible(), "Opening settings must retain the preview");
  await page.getByRole("switch", { name: "Autocorrect", exact: true }).click();
  await page.getByRole("switch", { name: "Adaptive key sizes", exact: true }).click();
  await page.getByRole("switch", { name: "Debug key sizes", exact: true }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  await reset("keynoard"); await space.click();
  assert(await value() === "keyboard ", "Autocorrect must correct keynoard to keyboard");
  await reset("hel");
  assert(await page.locator('[data-key-weight]').evaluateAll(elements => elements.some(element => Number(element.getAttribute("data-key-weight")) > 1)), "Predictions must enlarge likely next keys");
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
  await reset("first line\nsecond line\nthird line");
  await shift.click();
  await page.getByRole("button", { name: "Arrow left", exact: true }).click();
  assert(await fixture("context.selectionEnd - window.fixture.context.selectionStart") === 1, "Shift + arrow must select one character");
  await reset(Array.from({ length: 30 }, (_, i) => `Line ${i}`).join("\n"));
  const scrollable = page.locator("[data-preview-scroll]");
  assert(await scrollable.evaluate(element => element.scrollHeight > element.clientHeight), "Long previews must scroll");
  await scrollable.evaluate(element => { element.scrollTop = 0; });
  assert(await scrollable.evaluate(element => element.scrollTop) === 0, "Preview scroll must allow earlier text");
  await reset("hello\nworld", 0);
  await page.getByRole("button", { name: "Arrow down", exact: true }).click();
  assert(await fixture("context.selectionStart") === 6, "Down must follow preview line position");
  const point = await text.evaluate(element => { const range = document.createRange(); range.setStart(element.firstChild!, 2); range.setEnd(element.firstChild!, 3); const rect = range.getBoundingClientRect(); return { x: rect.x, y: rect.y + rect.height / 2 }; });
  await page.touchscreen.tap(point.x, point.y);
  assert(await fixture("context.selectionStart") === 2, "Tapping preview text must position the cursor");
  await page.screenshot({ path: "/tmp/keyboard-preview@2x.png" });
  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  await page.screenshot({ path: "/tmp/keyboard-settings@2x.png" });
  assert(errors.length === 0, `Browser errors: ${errors.join("; ")}`);
  console.log("PASS: navigation, period undo, suggestion spacing, blank lines, settings preview, autocorrect, adaptive keys, held Shift, selection and scrolling");
} finally { await browser.close(); await server.close(); }
