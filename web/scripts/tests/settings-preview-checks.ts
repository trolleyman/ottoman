import type { Page } from "playwright";

export async function checkSettingsPreview(page: Page, reset: (text: string, cursor?: number) => Promise<void>) {
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const configure = async (mirror: boolean, above: boolean) => {
    await page.evaluate(`window.fixture.configure(${JSON.stringify({ mirrorFocusedText: mirror, suggestionsAboveControls: above, keyboardTheme: "dark", tallerKeys: true, swipeTyping: false })})`);
    await page.waitForTimeout(60);
  };
  await page.evaluate("window.fixture.setOS('windows')");
  for (const viewport of [{ width: 402, height: 874 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(viewport);
    for (const mirror of [true, false]) for (const above of [true, false]) {
      await configure(mirror, above);
      if (mirror) await reset("Settings must keep this text preview visible.");
      else for (const letter of "hello") await page.locator(`[data-letter="${letter}"]`).click();
      await page.locator(".pc-remote").evaluate(element => { element.scrollTop = 0; });
      const preview = page.locator("[data-text-preview]");
      const bar = page.getByLabel("Word suggestions", { exact: true });
      const before = (await preview.boundingBox())!;
      const barBefore = (await bar.boundingBox())!;
      assert(before, "Both mirrored and locally typed text must have a preview");
      const style = (element: Element) => ({ background: getComputedStyle(element).backgroundColor, color: getComputedStyle(element).color, border: getComputedStyle(element).borderColor });
      const fnStyle = await page.getByRole("button", { name: "Function keys", exact: true }).evaluate(style);
      for (const name of ["Control", "Windows", "Alt", "Open desktop launcher"]) {
        assert(JSON.stringify(await page.getByRole("button", { name, exact: true }).evaluate(style)) === JSON.stringify(fnStyle), `${name} must match Fn in dark mode`);
      }
      const windows = page.getByRole("button", { name: "Windows", exact: true });
      await windows.click();
      assert(await windows.getAttribute("aria-pressed") === "true", "Windows must visibly activate on tap");
      assert(JSON.stringify(await windows.evaluate(style)) !== JSON.stringify(fnStyle), "Active Windows must not retain the inactive dark style");
      await page.locator('[data-letter="a"]').click();
      assert(await windows.getAttribute("aria-pressed") === "false", "Windows must clear after its shortcut");
      await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
      assert(await preview.isVisible() && await bar.isVisible(), "Settings must retain both text preview and suggestion strip");
      const after = (await preview.boundingBox())!;
      const barAfter = (await bar.boundingBox())!;
      assert(Math.abs(before.y - after.y) < 1 && Math.abs(before.height - after.height) < 1, `Opening settings must not move or resize the preview: ${JSON.stringify({ viewport, mirror, above, before, after })}`);
      assert(Math.abs(barBefore.y - barAfter.y) < 1, "Settings must not remove or shift the suggestion strip");
      assert(after.y >= 0 && after.y + after.height <= viewport.height, "The preview must actually stay on screen, not merely remain mounted");
      assert(await page.locator('[aria-label="Remote audio"]').getByRole("button", { name: /^(Mute|Unmute)/ }).count() === 0, "The side slider must not contain mute/unmute");
      if (mirror && above) await page.screenshot({ path: `/tmp/keyboard-settings-audit-${viewport.width}@2x.png` });
      await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
    }
  }
}
