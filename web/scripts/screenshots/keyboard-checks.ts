import type { Page } from "playwright";
import type { TrackpadMessage } from "../../src/api";

export interface SentMessage { message: TrackpadMessage; time: number }
function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

export async function checkKeyboardInteractions(page: Page, sent: SentMessage[]) {
  const key = (letter: string) => page.locator(`[data-letter="${letter}"]`);
  const textMessages = () => sent.filter(({ message }) => message.type === "text");
  const centre = async (letter: string) => {
    const box = await key(letter).boundingBox();
    assert(box, `Missing key ${letter}`);
    return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  };
  const fastText = "qwertyqwertyqwertyqwerty";
  const beforeFast = textMessages().length;
  await page.evaluate((text) => {
    // Preserve the browser method solely to restore it after synthetic events.
    // eslint-disable-next-line @typescript-eslint/unbound-method
    const capture = Element.prototype.setPointerCapture;
    Element.prototype.setPointerCapture = () => {};
    try {
      for (let index = 0; index < text.length; index++) {
        const key = document.querySelector<HTMLButtonElement>(`[data-letter="${text[index]}"]`)!;
        const box = key.getBoundingClientRect();
        const init = { bubbles: true, pointerId: index + 100, pointerType: "touch", isPrimary: true, button: 0, clientX: box.x + box.width / 2, clientY: box.y + box.height / 2 };
        key.dispatchEvent(new PointerEvent("pointerdown", { ...init, buttons: 1 }));
        key.dispatchEvent(new PointerEvent("pointerup", { ...init, buttons: 0 }));
      }
    } finally { Element.prototype.setPointerCapture = capture; }
  }, fastText);
  await page.waitForTimeout(80);
  const fastReceived = textMessages().slice(beforeFast).map(({ message }) => message.type === "text" ? message.text : "").join("");
  assert(fastReceived === fastText, `Rapid taps lost text: expected ${fastText}, received ${fastReceived}`);

  const beforeRollover = textMessages().length;
  await page.evaluate(() => {
    // Preserve the browser method solely to restore it after synthetic events.
    // eslint-disable-next-line @typescript-eslint/unbound-method
    const capture = Element.prototype.setPointerCapture;
    Element.prototype.setPointerCapture = () => {};
    const send = (letter: string, type: string, pointerId: number, isPrimary: boolean) => {
      const key = document.querySelector(`[data-letter="${letter}"]`)!;
      const box = key.getBoundingClientRect();
      key.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId, pointerType: "touch", isPrimary, button: 0, buttons: type === "pointerdown" ? 1 : 0, clientX: box.x + box.width / 2, clientY: box.y + box.height / 2 }));
    };
    try {
      for (let index = 0; index < 12; index++) {
        send("q", "pointerdown", 200 + index * 2, true);
        send("w", "pointerdown", 201 + index * 2, false);
        send("q", "pointerup", 200 + index * 2, true);
        send("w", "pointerup", 201 + index * 2, false);
      }
      send("e", "pointerdown", 400, true); send("e", "pointerdown", 401, false);
      send("e", "pointerup", 400, true); send("e", "pointerup", 401, false);
    } finally { Element.prototype.setPointerCapture = capture; }
  });
  await page.waitForTimeout(80);
  const rollover = textMessages().slice(beforeRollover).map(({ message }) => message.type === "text" ? message.text : "").join("");
  assert(rollover === "qw".repeat(12) + "ee", `Overlapping fingers lost text: ${rollover}`);
  await key("q").scrollIntoViewIfNeeded();
  const noGaps = await page.locator('[data-letter="q"]').evaluate((element) => {
    const box = element.getBoundingClientRect();
    const next = document.querySelector('[data-letter="w"]')!.getBoundingClientRect();
    return { gap: next.left - box.right, box: box.toJSON() as Record<string, number>, hit: document.elementFromPoint(box.right - 1, box.y + box.height / 2)?.closest('[data-letter]')?.getAttribute('data-letter'), height: innerHeight };
  });
  assert(Math.abs(noGaps.gap) < 0.1 && noGaps.hit === "q", `Letter key hit areas must meet at their edges: ${JSON.stringify(noGaps)}`);

  await page.getByRole("button", { name: "Windows", exact: true }).waitFor();
  assert(await page.evaluate(() => /Macintosh/.test(navigator.userAgent)), "Remote-OS test must use an Apple browser with a Windows agent");
  await key("e").scrollIntoViewIfNeeded();
  const start = await centre("e");
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.waitForTimeout(60);
  assert(await page.locator('[data-key-preview="letter"]').textContent() === "e", "Press popup must show the letter immediately");
  await page.waitForTimeout(450);
  assert(await page.locator('[data-key-preview="alternate"]').textContent() === "£", "Held E must preview sterling");
  const beforeHold = textMessages().length;
  await page.mouse.up();
  await page.waitForTimeout(60);
  const holdMessages = textMessages().slice(beforeHold);
  assert(holdMessages.length === 1 && holdMessages[0].message.type === "text" && holdMessages[0].message.text === "£", "Held key must emit only sterling on release");

  // A tap and a cancelled hold should never become a swipe or a late symbol.
  await key("h").dblclick();
  assert(await page.evaluate(() => getSelection()?.toString() === ""), "Double tapping keys must not select the page");
  assert(await key("h").evaluate((element) => getComputedStyle(element).userSelect) === "none", "Keyboard must disable text selection");
  const beforeCancel = textMessages().length;
  const cancelled = await centre("e");
  await page.mouse.move(cancelled.x, cancelled.y);
  await page.mouse.down();
  await key("e").dispatchEvent("pointercancel", { pointerId: 1, pointerType: "mouse", isPrimary: true });
  await page.mouse.up();
  await page.waitForTimeout(500);
  assert(textMessages().length === beforeCancel, "Cancelled press must not emit a delayed character");

  const beforeClear = sent.length;
  await page.getByRole("button", { name: "Clear preview", exact: true }).click();
  await page.waitForTimeout(60);
  assert(sent.length === beforeClear, "Clear preview must not send any remote input");
  for (const letter of "hel") await key(letter).click();
  const bar = page.getByLabel("Word suggestions");
  const completions = await bar.getByRole("button").allTextContents();
  assert(completions[0] === "hel" && completions.length > 1, `Word bar must preserve the literal prefix and offer completions: ${JSON.stringify(completions)}`);
  const beforeCompletion = sent.length;
  await bar.getByRole("button").nth(1).click();
  await page.waitForTimeout(60);
  assert(sent.slice(beforeCompletion).filter(({ message }) => message.type === "keydown" && message.key === "Backspace").length === 3, "Completion must replace only the typed prefix");
  await page.getByRole("button", { name: "Clear preview", exact: true }).click();

  const home = page.getByRole("button", { name: "Home", exact: true });
  const homeBox = await home.boundingBox();
  assert(homeBox, "Missing Home key");
  await page.mouse.move(homeBox.x + homeBox.width / 2, homeBox.y + homeBox.height / 2);
  await page.mouse.down();
  assert(await page.locator('[data-key-preview="action"]').textContent() === "Home", "Navigation keys must show a press preview");
  await page.mouse.up();
  await page.getByRole("button", { name: "More keyboard actions", exact: true }).click();
  const beforePrint = sent.length;
  await page.getByRole("button", { name: /Print screen/i }).click();
  await page.waitForTimeout(60);
  assert(sent.slice(beforePrint).some(({ message }) => message.type === "keydown" && message.key === "PrintScreen"), "More keys must send PrintScreen");

  // Every symbol should occupy exactly its hinted letter's rectangle.
  const rectangles = await page.locator("[data-letter]").evaluateAll((elements) => elements.map((element) => {
    const { x, y, width, height } = element.getBoundingClientRect();
    return { x, y, width, height, symbol: element.querySelector('.custom-keyboard-hint')?.textContent?.trim() };
  }));
  await page.getByRole("button", { name: "Symbols", exact: true }).click();
  const symbolRectangles = await page.locator(".custom-keyboard-key").evaluateAll((elements) => elements.slice(10, 36).map((element) => {
    const { x, y, width, height } = element.getBoundingClientRect();
    return { x, y, width, height, symbol: element.getAttribute('aria-label') };
  }));
  assert(JSON.stringify(rectangles) === JSON.stringify(symbolRectangles), `Symbol positions and sizes must match the letter hints: ${JSON.stringify({ rectangles, symbolRectangles })}`);
  await page.getByRole("button", { name: "Letters", exact: true }).click();

  await page.locator('[data-swipe-ready="true"]').waitFor();
  const beforeSwipe = textMessages().length;
  const hello = await Promise.all(Array.from("helo").map(centre));
  await page.mouse.move(hello[0].x, hello[0].y);
  await page.mouse.down();
  for (const point of hello.slice(1)) await page.mouse.move(point.x, point.y, { steps: 6 });
  await page.mouse.up();
  await page.waitForTimeout(100);
  const swiped = textMessages().slice(beforeSwipe);
  assert(swiped.length === 1 && swiped[0].message.type === "text" && swiped[0].message.text?.toLowerCase() === "hello", `Swipe must emit one decoded word without the initial letter: ${JSON.stringify(swiped)}`);
  await page.getByRole("button", { name: "hello", exact: true }).waitFor();

  const alternatives = await page.getByLabel("Word suggestions").getByRole("button").allTextContents();
  if (alternatives.length > 1) {
    const beforeCorrection = sent.length;
    await page.getByLabel("Word suggestions").getByRole("button", { name: alternatives[1], exact: true }).click();
    await page.waitForTimeout(60);
    const correction = sent.slice(beforeCorrection);
    assert(correction.filter(({ message }) => message.type === "keydown" && message.key === "Backspace").length === 5, "Alternative word must replace only the original word");
    assert(correction.some(({ message }) => message.type === "text" && message.text === alternatives[1]), "Chosen alternative must be inserted");
  }
  const touchPath = await Promise.all(Array.from("world").map(centre));
  const beforeTouchSwipe = textMessages().length;
  const touch = await page.context().newCDPSession(page);
  try {
    await touch.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ ...touchPath[0], id: 1 }] });
    for (let index = 1; index < touchPath.length; index++) {
      const from = touchPath[index - 1];
      const to = touchPath[index];
      for (let step = 1; step <= 6; step++) await touch.send("Input.dispatchTouchEvent", {
        type: "touchMove", touchPoints: [{ x: from.x + (to.x - from.x) * step / 6, y: from.y + (to.y - from.y) * step / 6, id: 1 }],
      });
    }
    await touch.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await page.waitForTimeout(100);
    const touchWords = textMessages().slice(beforeTouchSwipe);
    assert(touchWords.length === 1 && touchWords[0].message.type === "text" && touchWords[0].message.text === " world", `Touch swipe must emit exactly one decoded word: ${JSON.stringify(touchWords)}`);
  } finally { await touch.detach(); }

  await page.getByRole("button", { name: "Comma", exact: true }).evaluate((element) => {
    const target = element as HTMLElement & { capturedEvents: string[] };
    target.capturedEvents = [];
    for (const type of ["pointerdown", "pointerup", "pointercancel", "click"])
      target.addEventListener(type, (event) => target.capturedEvents.push(`${event.type}:${(event as PointerEvent).pointerType}`));
  });
  const beforeComma = textMessages().length;
  const commaBox = await page.getByRole("button", { name: "Comma", exact: true }).boundingBox();
  assert(commaBox, "Missing direct comma key");
  await page.touchscreen.tap(commaBox.x + commaBox.width / 2, commaBox.y + commaBox.height / 2);
  await page.waitForTimeout(300);
  const commaEvents = await page.getByRole("button", { name: "Comma", exact: true }).evaluate((element) => (element as HTMLElement & { capturedEvents: string[] }).capturedEvents);
  assert(textMessages().slice(beforeComma).some(({ message }) => message.type === "text" && message.text === ","), `A real touch tap on the comma key must type a comma: ${JSON.stringify({ messages: textMessages().slice(beforeComma), events: commaEvents })}`);

  // Preview stays until another interaction, rather than fading after a timer.
  await page.waitForTimeout(4700);
  assert(await page.locator("[data-text-preview]").isVisible(), "Typing preview must not disappear on a timer");
  const backspace = page.getByRole("button", { name: "Backspace", exact: true });
  const box = await backspace.boundingBox();
  assert(box, "Missing backspace");
  const beforeErase = sent.length;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.waitForTimeout(2100);
  await page.mouse.up();
  await page.waitForTimeout(60);
  const erases = sent.slice(beforeErase).filter(({ message }) => message.type === "keydown" && message.key === "Backspace");
  assert(erases.length > 35, "Backspace hold must repeat");
  const average = (values: SentMessage[]) => (values[values.length - 1].time - values[0].time) / (values.length - 1);
  assert(average(erases.slice(-8)) < average(erases.slice(2, 10)) * 0.8, "Backspace repeat must accelerate");

  const rail = page.getByRole("group", { name: "One-finger scroll control" });
  const railBox = await rail.boundingBox();
  assert(railBox, "Missing scroll rail");
  await page.mouse.move(railBox.x + railBox.width / 2, railBox.y + railBox.height * 0.7);
  await page.mouse.down();
  await page.mouse.move(railBox.x + railBox.width / 2, railBox.y + railBox.height * 0.4, { steps: 5 });
  assert(await rail.getAttribute("data-scroll-phase") === "dragging", "Scroll rail must show drag feedback");
  await page.mouse.up();
  const scrollCount = sent.filter(({ message }) => message.type === "mousescroll").length;
  await page.waitForTimeout(150);
  assert(sent.filter(({ message }) => message.type === "mousescroll").length > scrollCount, "Rail must continue scrolling after release");
  assert(!await page.locator("[data-text-preview]").isVisible(), "Scrolling must dismiss the typing preview");

  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  await page.getByLabel("Keyboard appearance").selectOption("light");
  assert(await page.locator(".custom-keyboard-shell").getAttribute("data-keyboard-theme") === "light", "Explicit light theme must apply");
  await page.getByLabel("Keyboard appearance").selectOption("dark");
  await page.getByRole("switch", { name: "Period beside space" }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  assert(await key("e").isVisible(), "Back from settings must keep the keyboard open");
  assert(await page.getByRole("button", { name: "Period", exact: true }).count() === 0, "Period toggle must remove the bottom-row key");
}
