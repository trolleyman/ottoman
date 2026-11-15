import type { Page } from "playwright";

export async function checkPhoneText(page: Page, out: string) {
  const assert = (value: unknown, message: string) => { if (!value) throw Error(message); };
  const fixture = (code: string) => page.evaluate(`window.fixture.${code}`);
  const field = page.getByLabel("Remote text", { exact: true });
  const longText = Array.from({ length: 16 }, (_, i) => `Line ${i + 1}: words that wrap on the phone screen.`).join("\n");
  for (const mode of ["native", "local", "rich"]) {
    const native = mode === "native", available = mode !== "local";
    await fixture(`setField(${JSON.stringify({ available, editable: native, value: available ? longText : "", selectionStart: 0, selectionEnd: 0 })})`);
    await page.waitForTimeout(80);
    if (!available) { await page.locator('[data-letter="q"]').click(); await field.fill(longText); }
    await field.evaluate((element: HTMLTextAreaElement) => { element.focus(); element.setSelectionRange(0, 0); document.dispatchEvent(new Event("selectionchange")); });
    await page.waitForTimeout(50);
    await fixture("log.length = 0");
    await page.getByRole("button", { name: "Arrow down", exact: true }).click();
    const down = await field.evaluate((element: HTMLTextAreaElement) => element.selectionEnd);
    assert(down > 0 && down < longText.indexOf("\n"), `Down must follow the phone's wrapped line (${mode}): ${JSON.stringify({ down, value: await field.inputValue(), log: await fixture("log"), context: await fixture("context") })}`);
    await page.getByRole("button", { name: "Home", exact: true }).click();
    const home = await field.evaluate((element: HTMLTextAreaElement) => element.selectionEnd);
    await page.getByRole("button", { name: "End", exact: true }).click();
    const end = await field.evaluate((element: HTMLTextAreaElement) => element.selectionEnd);
    assert(end > home && end < longText.indexOf("\n") + 1, "Home/End must use the phone's wrapped line");
    const messages = await fixture("log") as { type: string; key?: string }[];
    assert(!messages.some(message => message.type === "keydown" && /^(ArrowUp|ArrowDown|Home|End)$/.test(message.key || "")), "Phone navigation must not send host line-navigation keys");
    await field.evaluate((element: HTMLTextAreaElement) => { element.focus(); element.setSelectionRange(1, 6); document.dispatchEvent(new Event("selectionchange")); });
    await page.waitForTimeout(50);
    if (available) assert((await fixture("context.selectionStart")) === 1 && (await fixture("context.selectionEnd")) === 6, "Document selection changes must synchronize iOS-style selection handles");
    assert(await field.evaluate((element: HTMLTextAreaElement) => element.selectionStart === 1 && element.selectionEnd === 6), "Native selection must remain stable");
    await field.evaluate((element: HTMLTextAreaElement) => { element.setSelectionRange(0, 0); document.dispatchEvent(new Event("selectionchange")); element.scrollTop = 0; });
    await page.waitForTimeout(40);
    assert(await field.getAttribute("data-preview-hidden-above") === null && await field.getAttribute("data-preview-hidden-below") !== null, "Only overflow below must fade at the top");
    assert(await page.locator(".native-text-frame").evaluate(element => getComputedStyle(element).maskImage) === "none", "Text fading must never fade the frame");
    const box = (await field.boundingBox())!;
    const cdp = await page.context().newCDPSession(page);
    const x = box.x + box.width / 2, y = box.y + box.height - 8;
    await fixture("log.length = 0");
    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y, id: 1 }] });
    for (let i = 1; i <= 8; i++) {
      await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y: y - i * 5, id: 1 }] });
      await page.waitForTimeout(16);
    }
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await cdp.detach();
    await page.waitForTimeout(150);
    assert(await field.evaluate((element: HTMLTextAreaElement) => element.scrollTop > 0), "Finger swipes must scroll the native text box");
    assert(!(await fixture('log') as { type: string }[]).some(message => /^(mouse|touch)/.test(message.type)), "Selecting and scrolling text must never control the trackpad");
    await field.evaluate((element: HTMLTextAreaElement) => { element.scrollTop = element.scrollHeight; });
    await page.waitForTimeout(40);
    assert(await field.getAttribute("data-preview-hidden-above") !== null && await field.getAttribute("data-preview-hidden-below") === null, "Bottom fade must disappear at the bottom");
    assert(await field.evaluate((element: HTMLTextAreaElement) => element.style.maskImage.includes("8px")), "Overflow fades must be compact");
    await field.evaluate((element: HTMLTextAreaElement) => { element.focus(); element.setSelectionRange(0, 0); document.dispatchEvent(new Event("selectionchange")); element.scrollTop = 0; });
    const space = page.getByRole("button", { name: "Space. Hold and drag to move the cursor", exact: true });
    const key = (await space.boundingBox())!;
    await page.mouse.move(key.x + key.width / 2, key.y + key.height / 2); await page.mouse.down();
    try {
      await page.waitForTimeout(380);
      const ghost = page.locator(".native-caret-overlay [data-drag-caret]");
      await ghost.waitFor();
      const origin = (await ghost.boundingBox())!;
      assert(origin.y >= box.y && origin.y + origin.height <= box.y + box.height, "Floating caret must sit inside the visible text field");
      assert(await ghost.evaluate((element: HTMLElement) => Number(getComputedStyle(element.parentElement!).zIndex) > Number(getComputedStyle(document.querySelector('[data-native-preview]')!).zIndex)), "Floating caret must render above the textarea");
      await page.mouse.move(key.x + key.width / 2 + 20, key.y + key.height / 2);
      assert((await ghost.boundingBox())!.x > origin.x, "Spacebar drag must move the floating caret in mirrored and local text");
      if (!native) await page.screenshot({ path: `${out}/local-cursor@2x.png` });
    } finally { await page.mouse.up(); }
    assert(await page.locator("[data-drag-caret]").count() === 0, "Releasing space removes the floating caret");
    const pad = page.getByLabel(/Trackpad\. Tap to click/);
    const padBox = (await pad.boundingBox())!;
    await field.fill("one");
    await page.waitForTimeout(40);
    assert((await field.boundingBox())!.height === 42, "A single line must use a single-line field");
    assert(await field.evaluate((element: HTMLTextAreaElement) => element.scrollHeight === element.clientHeight), "One line must not have a tiny scroll range");
    await field.fill("one\ntwo");
    await page.waitForTimeout(40);
    assert((await field.boundingBox())!.height === 66, "Two complete lines must fit with padding and borders");
    assert(await field.evaluate((element: HTMLTextAreaElement) => element.scrollHeight === element.clientHeight), "Two lines must not have a tiny scroll range");
    await field.fill(Array.from({ length: 12 }, (_, index) => `line ${index}`).join("\n"));
    await field.evaluate((element: HTMLTextAreaElement) => { element.scrollTop = 0; });
    await page.locator('[data-letter="q"]').click();
    await page.waitForTimeout(50);
    assert(await field.evaluate((element: HTMLTextAreaElement) => element.scrollTop > 0 && element.scrollHeight - element.clientHeight - element.scrollTop < 2), "Typing with the custom keyboard must follow the caret at the end");
    assert(JSON.stringify(await pad.boundingBox()) === JSON.stringify(padBox), "Fitting the field must not resize the trackpad");

    if (mode === "rich") {
      await fixture("log.length = 0");
      await field.fill("Rich edit 👩‍💻 stays whole");
      await page.waitForTimeout(60);
      assert(await fixture("context.value") === "Rich edit 👩‍💻 stays whole", "Native phone edits must reach keystroke-only rich host fields");
      assert(!(await fixture("log") as { type: string }[]).some(message => message.type === "textedit"), "Rich field must keep its supported keystroke path");
    }
  }
  await fixture('reset("Select and edit this text normally.")');
  console.log("PASS: native selection, touch scrolling, phone-relative navigation and visible spacebar caret for native, local and keystroke-only rich text");
}
