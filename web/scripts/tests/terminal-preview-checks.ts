import { openSettings } from "./settings-helpers";
import type { Page } from "playwright";

export async function checkTerminalPreview(page: Page) {
  await page.setViewportSize({ width: 402, height: 874 });
  await page.waitForTimeout(80);
  await openSettings(page);
  if (await page.getByRole("switch", { name: "Show terminal scrollback", exact: true }).getAttribute("aria-checked") !== "false") throw Error("Terminal scrollback must default off");
  if (await page.getByLabel("Terminal font", { exact: true }).inputValue() !== "iosevka") throw Error("Terminal font must default to Iosevka");
  await page.getByRole("switch", { name: "Show terminal scrollback", exact: true }).click();
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  const value = "~/code/ottoman \ue0a0 main\n❯ echo 'hello world' \uf120";
  await page.evaluate(value => {
    const colorRun = (text: string, color: string) => {
      const start = Array.from(value.slice(0, value.indexOf(text))).length;
      return { start, end: start + Array.from(text).length, color };
    };
    (window as unknown as { fixture: { setField: (v: object) => void } }).fixture.setField({ terminal: true, editable: false, value, selectionStart: Array.from(value).length, selectionEnd: Array.from(value).length, terminalColors: [colorRun("~/code/ottoman", "#00cdd8"), colorRun("main", "#e879f9"), colorRun("❯", "#4ade80")] });
  }, value);
  const preview = page.locator("[data-terminal-preview]");
  await preview.waitFor();
  await page.evaluate(() => document.fonts.ready);
  if (!await preview.evaluate(element => getComputedStyle(element).fontFamily.startsWith('"Iosevka Term Nerd Font Mono"'))) throw Error("Default Nerd Font not applied");
  const nerdFontLoaded = await page.evaluate(async () => {
    const fonts = await document.fonts.load('14px "Iosevka Term Nerd Font Mono"', "\ue0a0\uf120\u{f0001}");
    const canvas = document.createElement("canvas");
    const context = canvas.getContext("2d")!;
    context.font = '14px "Iosevka Term Nerd Font Mono"';
    const cell = context.measureText("M").width;
    return fonts.length > 0 && fonts.every(font => font.status === "loaded") &&
      ["\ue0a0", "\uf120", "\u{f0001}"].every(icon => Math.abs(context.measureText(icon).width - cell) < 0.01);
  });
  if (!nerdFontLoaded) throw Error("Bundled Nerd Font must load and retain single-cell BMP and supplementary icons");
  if (await page.locator("[data-terminal-color]").count() === 0) throw Error("Terminal colours were not applied");
  const wholeBranchHighlighted = await page.locator("[data-terminal-color]").evaluateAll(elements => elements.some(element =>
    element.textContent === "main" && getComputedStyle(element).color === "rgb(232, 121, 249)"));
  if (!wholeBranchHighlighted) throw Error("Branch colour must include the entire name, including the first letter");
  const viewport = page.locator("[data-preview-scroll]");
  if (await viewport.getAttribute("data-preview-hidden-above")) throw Error("Short terminal preview must not fade");
  await page.locator("[data-text-preview]").screenshot({ path: "/tmp/terminal-command@2x.png" });
  await preview.screenshot({ path: "/tmp/terminal-nerd-font@2x.png" });
  await openSettings(page);
  await page.getByLabel("Terminal font", { exact: true }).selectOption("fira-code");
  await page.getByRole("button", { name: "Back to keyboard", exact: true }).click();
  if (!await preview.evaluate(element => getComputedStyle(element).fontFamily.includes("Fira Code"))) throw Error("Selected font not applied");
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
