import type { Page } from "playwright";
export async function checkImagePaste(page: Page) {
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const clipboard = page.getByRole("button", { name: "Clipboard", exact: true });
  const keyboard = page.locator(".custom-keyboard-shell");
  const beforeBox = (await keyboard.boundingBox())!;
  await clipboard.click();
  const photo = page.getByRole("button", { name: "Paste image", exact: true });
  assert(await photo.isVisible(), "Clipboard must reuse the function-key row");
  assert((await keyboard.boundingBox())!.height === beforeBox.height, "Clipboard actions must not increase keyboard height");
  const png = Buffer.from(await page.evaluate(() => { const canvas = document.createElement("canvas"); canvas.width = 16; canvas.height = 16; return canvas.toDataURL().split(",")[1]; }), "base64");
  const keydowns = () => page.evaluate(() => (window as unknown as { fixture: { log: { type: string; key?: string; modifiers?: string[] }[] } }).fixture.log.filter(message => message.type === "keydown" && message.key === "v"));
  let release: (() => void) | undefined;
  let fail = false;
  let uploads = 0;
  let started: () => void = () => {};
  const uploadStarted = new Promise<void>(resolve => { started = resolve; });
  await page.route("**/api/clipboard/image", async route => {
    uploads++; started();
    assert(route.request().headers()["content-type"] === "image/png", "Upload must be PNG");
    assert(route.request().postDataBuffer()?.subarray(1, 4).toString() === "PNG", "Upload must contain real image bytes");
    if (fail) { await route.fulfill({ status: 503, body: "Desktop clipboard unavailable" }); return; }
    await new Promise<void>(resolve => { release = resolve; });
    await route.fulfill({ status: 204 });
  });
  const choose = async () => { const chooser = page.waitForEvent("filechooser"); await photo.click(); await (await chooser).setFiles({ name: "photo.png", mimeType: "image/png", buffer: png }); };
  const before = (await keydowns()).length;
  await choose();
  await page.getByRole("status").filter({ hasText: "Sending image" }).waitFor();
  await page.waitForFunction(() => Boolean(document.querySelector('button[aria-label="Paste image"]:disabled')));
  await uploadStarted;
  assert(uploads === 1 && (await keydowns()).length === before, "Image upload must start immediately but paste must wait for acknowledgement");
  assert(await page.getByRole("button", { name: "Paste", exact: true }).count() === 0, "Image paste must not ask for confirmation");
  release!();
  await photo.waitFor({ state: "hidden" });
  assert(await clipboard.getAttribute("aria-pressed") === "false", "Successful image paste must deselect Clipboard");
  await clipboard.click();
  assert((await keydowns()).length === before + 1, "Successful image upload must send one paste shortcut");
  fail = true; await choose();
  await page.getByRole("alert").filter({ hasText: "Desktop clipboard unavailable" }).waitFor();
  assert((await keydowns()).length === before + 1, "Failed upload must never paste stale clipboard data");
  await page.getByRole("button", { name: "Dismiss image error" }).click();
  await page.unroute("**/api/clipboard/image");
  await clipboard.click();
  console.log("PASS: fixed-height clipboard row, immediate image upload, acknowledgement and error handling");
}
