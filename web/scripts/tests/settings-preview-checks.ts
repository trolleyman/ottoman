import type { Page } from "playwright";
import { openSettings } from "./settings-helpers";
export async function checkSettingsPreview(page: Page, reset: (text: string, cursor?: number) => Promise<void>) {
  for (const viewport of [{ width: 402, height: 874 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(viewport);
    await page.evaluate("window.fixture.configure({mirrorFocusedText:true,swipeTyping:false})");
    await page.waitForTimeout(70); await reset("Settings preserve this field.");
    const field = page.getByLabel("Remote text", { exact: true });
    const pad = page.getByLabel(/Trackpad\. Tap to click/);
    const before = (await pad.boundingBox())!;
    await openSettings(page);
    if (await field.inputValue() !== "Settings preserve this field.") throw Error("Settings must preserve the remote text");
    await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
    const after = (await pad.boundingBox())!;
    if (JSON.stringify(before) !== JSON.stringify(after)) throw Error("Opening settings must not permanently resize the trackpad");
    const box = (await field.boundingBox())!;
    if (box.height > 65 || box.x < 0 || box.x + box.width > viewport.width) throw Error("Native field must remain two lines wide within the viewport");
    await page.getByRole("button", { name: "Dismiss preview", exact: true }).click();
    const dismissed = (await pad.boundingBox())!;
    if (JSON.stringify(after) !== JSON.stringify(dismissed)) throw Error("Dismissing the field must not resize the trackpad");
    if (await page.locator('.custom-keyboard-shell [aria-label="Keyboard settings"]').count()) throw Error("Keyboard must not contain a settings button");
  }
}
