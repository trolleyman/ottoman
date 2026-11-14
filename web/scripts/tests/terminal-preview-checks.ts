import type { Page } from "playwright";

export async function checkTerminalPreview(page: Page) {
  await page.setViewportSize({ width: 402, height: 874 });
  await page.evaluate("window.fixture.configure({interactivePreview:true})");
  await page.waitForTimeout(80);
  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  if (await page.getByRole("switch", { name: "Show terminal scrollback", exact: true }).getAttribute("aria-checked") !== "false") throw Error("Terminal scrollback must default off");
  if (await page.getByLabel("Terminal font", { exact: true }).inputValue() !== "iosevka") throw Error("Terminal font must default to Iosevka");
  await page.getByRole("switch", { name: "Show terminal scrollback", exact: true }).click();
  await page.getByLabel("Terminal font", { exact: true }).selectOption("fira-code");
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  const value = "~/code/ottoman on main\n❯ echo 'hello world'";
  await page.evaluate(value => {
    (window as unknown as { fixture: { setField: (v: object) => void } }).fixture.setField({ terminal: true, editable: false, value, selectionStart: Array.from(value).length, selectionEnd: Array.from(value).length, terminalColors: [{ start: 0, end: 14, color: "#00cdd8" }, { start: 18, end: 22, color: "#e879f9" }, { start: 23, end: 24, color: "#4ade80" }] });
  }, value);
  const preview = page.locator("[data-terminal-preview]");
  await preview.waitFor();
  await page.evaluate(() => document.fonts.ready);
  if (!await preview.evaluate(element => getComputedStyle(element).fontFamily.includes("Fira Code"))) throw Error("Selected font not applied");
  if (!await page.evaluate(() => CSS.highlights.size > 0)) throw Error("Terminal colours were not applied");
  const viewport = page.locator("[data-preview-scroll]");
  if (await viewport.getAttribute("data-preview-hidden-above")) throw Error("Short terminal preview must not fade");
  await page.locator("[data-text-preview]").screenshot({ path: "/tmp/terminal-command@2x.png" });
  const longValue = Array.from({ length: 35 }, (_, i) => `build step ${i + 1}: completed`).join("\n") + "\n" + value;
  await page.evaluate(value => {
    (window as unknown as { fixture: { setField: (v: object) => void } }).fixture.setField({ terminal: true, editable: false, value, selectionStart: value.length, selectionEnd: value.length, terminalColors: [] });
  }, longValue);
  await page.waitForTimeout(100);
  if (await viewport.getAttribute("data-preview-hidden-above") !== "true") throw Error("Overflow above the caret must show a top fade");
  await page.locator("[data-text-preview]").screenshot({ path: "/tmp/terminal-scrollback@2x.png" });
  await viewport.evaluate(element => { element.scrollTop = 0; });
  await page.waitForTimeout(50);
  if (await viewport.getAttribute("data-preview-hidden-above")) throw Error("Fade must disappear at the top of scrollback");
  const saved = await page.evaluate(() => JSON.parse(localStorage.getItem("ottoman-trackpad-settings")!) as { terminalFont: string; terminalScrollback: boolean });
  if (saved.terminalFont !== "fira-code" || !saved.terminalScrollback) throw Error("Terminal preferences were not saved");
  console.log("PASS: terminal fonts, colours, scrollback preference and overflow fade");
}
