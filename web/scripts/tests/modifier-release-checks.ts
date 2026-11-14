import type { Page } from "playwright";
import { Modifier, type TrackpadMessage } from "../../src/api";

export async function checkModifierRelease(page: Page) {
  const fixture = (expression: string) => page.evaluate(`window.fixture.${expression}`);
  await fixture('setField({ fieldId: "terminal", available: false, editable: false })');
  const ctrl = page.getByRole("button", { name: "Control", exact: true });
  const enter = page.getByRole("button", { name: "Enter", exact: true });
  const trackpad = page.getByLabel(/Trackpad\. Tap to click/);
  const assertControl = async (pressed: boolean) => {
    // Replay the agent's modifier snapshots and explicit Control events, then
    // check what the remote OS would see before the next key/click repairs it.
    const messages = await fixture("log") as TrackpadMessage[];
    let control = false;
    for (const message of messages) {
      if ("modifiers" in message) control = message.modifiers.includes(Modifier.CTRL);
      if ((message.type === "keydown" || message.type === "keyup") && message.key === "Control") control = message.type === "keydown";
    }
    if (control !== pressed) throw Error(`Remote Control must be ${pressed}: ${JSON.stringify(messages)}`);
    if (await ctrl.getAttribute("aria-pressed") !== String(pressed)) throw Error("Control selection must match the remote state");
  };
  const scroll = async () => {
    await trackpad.hover();
    await page.mouse.wheel(0, 120);
    await page.waitForTimeout(50);
    if (!(await fixture("log") as TrackpadMessage[]).some(message => message.type === "mousescroll")) throw Error("Expected a remote scroll");
  };
  for (let i = 0; i < 3; i++) {
    await ctrl.click();
    await enter.click();
    await scroll();
    await assertControl(false);
  }
  await ctrl.dblclick();
  await enter.click();
  await scroll();
  await assertControl(true);
  await ctrl.click();
  await scroll();
  await assertControl(false);

  const pointer = async (type: string) => ctrl.evaluate((element, type) => {
    const capture = element.setPointerCapture.bind(element);
    element.setPointerCapture = () => {};
    element.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId: 901, pointerType: "touch", button: 0 }));
    element.setPointerCapture = capture;
  }, type);
  await pointer("pointerdown");
  await enter.click();
  await assertControl(true);
  await pointer("pointerup");
  await assertControl(false);
  await pointer("pointerdown");
  await enter.click();
  await pointer("pointercancel");
  await assertControl(false);
  await pointer("pointerdown");
  await enter.click();
  await page.evaluate(() => window.dispatchEvent(new Event("blur")));
  await assertControl(false);
  await fixture('setField({ fieldId: "fixture" })');
  console.log("PASS: Ctrl+Enter releases one-shot modifiers before scrolling; locked, held, cancelled and blurred Control stay in sync");
}
