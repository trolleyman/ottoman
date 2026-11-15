import type { Page } from "playwright";
export async function openSettings(page: Page) {
  const settings = page.getByRole("button", { name: "Keyboard settings", exact: true });
  if (!await settings.isVisible()) await page.getByRole("button", { name: "Open menu", exact: true }).click();
  await settings.click();
  await page.locator(".input-settings details").evaluateAll(elements => elements.forEach(element => { (element as HTMLDetailsElement).open = true; }));
}
export async function openMenu(page: Page) {
  if (!await page.getByRole("navigation", { name: "Remote navigation" }).isVisible()) await page.getByRole("button", { name: "Open menu", exact: true }).click();
}
export async function closeMenu(page: Page) {
  if (await page.getByRole("navigation", { name: "Remote navigation" }).isVisible()) await page.getByRole("button", { name: "Close menu", exact: true }).last().click();
}
