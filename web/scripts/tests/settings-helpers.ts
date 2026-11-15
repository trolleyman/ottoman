import type { Page } from "playwright";
async function settleMenu(page: Page) {
  await page.locator("#remote-menu").evaluate(async element => {
    await Promise.all(element.getAnimations().map(animation => animation.finished.catch(() => {})));
  });
}
export async function openSettings(page: Page) {
  await openMenu(page);
  await page.getByRole("button", { name: "Keyboard settings", exact: true }).click();
  await page.locator(".input-settings details").evaluateAll(elements => elements.forEach(element => { (element as HTMLDetailsElement).open = true; }));
  await settleMenu(page);
}
export async function openMenu(page: Page) {
  await settleMenu(page);
  if (await page.locator("#remote-menu").getAttribute("data-open") !== "true") await page.getByRole("button", { name: "Open menu", exact: true }).click();
  await page.waitForFunction(() => document.querySelector("#remote-menu")?.getAttribute("data-open") === "true");
  await settleMenu(page);
}
export async function closeMenu(page: Page) {
  await settleMenu(page);
  if (await page.locator("#remote-menu").getAttribute("data-open") === "true") await page.getByRole("button", { name: "Close menu", exact: true }).last().click();
  await page.waitForFunction(() => document.querySelector("#remote-menu")?.getAttribute("data-open") === "false");
  await settleMenu(page);
}
