import type { Page } from "playwright";

export async function checkPredictionAndCursor(page: Page, reset: (text: string, cursor?: number) => Promise<void>) {
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const key = (letter: string) => page.locator(`[data-letter="${letter}"]`);
  const bar = page.getByLabel("Word suggestions", { exact: true });
  for (const [text, expected] of [["thank ", "you"], ["THANK ", "YOU"], ["i would like ", "to"], ["We paint turquoise tiles. We paint t", "turquoise"]]) {
    await reset(text);
    assert(await bar.getByRole("button").first().textContent() === (text.endsWith(" ") ? expected : "t"), "Literal prefixes must remain available");
    assert((await bar.getByRole("button").allTextContents()).includes(expected), `Context must predict ${expected} after ${text}`);
  }
  await reset("unfamiliar words ");
  assert(await bar.getByRole("button").count() === 3, "Next-word suggestions must have a frequency fallback");
  assert(await bar.locator("[aria-pressed], svg").count() === 0, "Suggestions must not show correction ticks or confirmation highlighting");

  const rectangles = () => page.locator("[data-letter]").evaluateAll(elements => elements.map(element => {
    const rect = element.getBoundingClientRect();
    return { letter: element.getAttribute("data-letter"), scale: Number(element.getAttribute("data-key-scale")), x: rect.x + rect.width / 2, y: rect.y + rect.height / 2, width: rect.width, height: rect.height };
  }));
  await reset("hel");
  const enlarged = await rectangles();
  const grown = enlarged.filter(point => point.scale > 1).sort((a, b) => b.scale - a.scale)[0];
  assert(grown, "The prefix must enlarge at least one predicted key");
  const edgeHits = await page.evaluate(({ x, y }) => document.elementFromPoint(x, y)?.closest("[data-letter]")?.getAttribute("data-letter"), { x: grown.x, y: grown.y - grown.height / 2 + 0.5 });
  assert(edgeHits === grown.letter, `The enlarged vertical edge must belong to the enlarged key: ${String(edgeHits)}, ${JSON.stringify(grown)}`);
  await page.screenshot({ path: "/tmp/adaptive-keyboard@2x.png" });
  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  await page.getByRole("switch", { name: "Adaptive key sizes", exact: true }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  const normal = await rectangles();
  for (let i = 0; i < normal.length; i++) assert(Math.abs(normal[i].x - enlarged[i].x) < 0.1 && Math.abs(normal[i].y - enlarged[i].y) < 0.1, "Predictions must not move any key center");
  const original = normal.find(point => point.letter === grown.letter)!;
  assert(grown.width > original.width && grown.height > original.height, "Keys must grow in both dimensions");
  assert(Math.abs(grown.width / original.width - grown.height / original.height) < 0.01, "Key growth must use the same scale horizontally and vertically");
  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  await page.getByRole("switch", { name: "Adaptive key sizes", exact: true }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  await reset("HEL");
  assert(Number(await key(grown.letter!).getAttribute("data-key-scale")) > 1, "Uppercase prefixes must still enlarge predicted letters");
  const shift = page.getByRole("button", { name: "Shift", exact: true });
  for (const button of [shift, page.getByRole("button", { name: /^(Super|Windows|Command)$/ })]) {
    const color = await button.evaluate(element => ({ background: getComputedStyle(element).backgroundColor, text: getComputedStyle(element).color }));
    assert((color.background === "rgb(39, 39, 42)" || color.background === "oklch(0.274 0.006 286.033)") && (color.text === "rgb(244, 244, 245)" || color.text === "oklch(0.967 0.001 286.375)"), `Inactive modifiers must match the readable utility keys: ${JSON.stringify(color)}`);
  }
  await shift.click();
  assert(await key(grown.letter!).locator(":scope > span.relative").textContent() === grown.letter!.toUpperCase(), "Shift must show the uppercase predicted letter");
  await key(grown.letter!).click();

  for (const value of ["word", "office", "W", "hello\nword", "emoji 👍", "é", "word.", "word ", "word\n"]) {
    await reset(value);
    const distance = await page.locator("[data-text-preview]").evaluate(element => {
      const span = element.querySelector("[data-caret-text]")!;
      const node = span.firstChild!;
      const value = node.textContent!.slice(0, -1);
      const range = document.createRange();
      range.setStart(node, value.endsWith("\n") ? value.length : value.length - Array.from(value).at(-1)!.length);
      range.setEnd(node, value.length + (value.endsWith("\n") ? 1 : 0));
      const rect = range.getBoundingClientRect();
      const caret = element.querySelector("[data-preview-caret]")!.getBoundingClientRect();
      return Math.abs(caret.left - (value.endsWith("\n") ? rect.left : rect.right));
    });
    assert(distance < 1, `End caret must follow the final character: ${JSON.stringify(value)}, distance ${distance}`);
  }
  await reset("abcdefghij", 0);
  await page.evaluate("window.savedCaret = document.querySelector('[data-preview-caret]')");
  await page.getByRole("button", { name: "Arrow right", exact: true }).click();
  assert(await page.evaluate("window.savedCaret === document.querySelector('[data-preview-caret]')"), "Cursor movement must reuse the same caret element");
  assert(await page.locator("[data-preview-caret]").evaluate(element => getComputedStyle(element).animationName) === "none", "Caret must not blink during cursor movement");
  await reset("abcdefghij", 0);
  const space = page.getByRole("button", { name: "Space. Hold and drag to move the cursor", exact: true });
  const box = (await space.boundingBox())!;
  const start = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  await page.mouse.move(start.x, start.y); await page.mouse.down();
  try {
    await page.waitForTimeout(380);
    const ghost = page.locator("[data-drag-caret]");
    const origin = (await ghost.boundingBox())!;
    assert(await page.locator("[data-preview-caret]").evaluate(element => getComputedStyle(element).backgroundColor) === "rgb(161, 161, 170)", "Actual caret must be gray while dragging");
    await page.mouse.move(start.x + 1, start.y);
    const fractional = (await ghost.boundingBox())!;
    assert(fractional.x > origin.x && fractional.x - origin.x < 4, "The blue caret must move smoothly before crossing a character boundary");
    await page.mouse.move(start.x + 24, start.y, { steps: 6 });
    await page.screenshot({ path: "/tmp/keyboard-cursor-drag@2x.png" });
    await page.waitForTimeout(500);
    assert(await page.locator("[data-preview-caret]").evaluate(element => getComputedStyle(element).animationName) === "none", "Dragging must keep the actual caret continuously visible");
  } finally { await page.mouse.up(); }
  assert(await page.locator("[data-drag-caret]").count() === 0, "Releasing space must remove the floating caret");
  assert(await page.locator("[data-preview-caret]").getAttribute("data-caret-parked") === null, "Releasing space must restore the actual caret color");
}
