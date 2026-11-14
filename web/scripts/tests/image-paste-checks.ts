import type { Page } from "playwright";

export async function checkImagePaste(page: Page) {
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const settings = page.getByRole("button", { name: "Keyboard settings", exact: true });
  const fn = page.getByRole("button", { name: "Function keys", exact: true });
  const photo = page.getByRole("button", { name: "Paste image", exact: true });
  const controls = page.getByLabel("Keyboard controls", { exact: true });
  const originalBox = await settings.boundingBox();
  const originalCount = await controls.getByRole("button").count();
  assert(await settings.isVisible() && !await photo.isVisible(), "Normal keyboard must show only settings");
  await fn.click();
  assert(await photo.isVisible() && !await settings.isVisible(), "Fn must swap settings for an image icon");
  assert(await controls.getByRole("button").count() === originalCount, "Fn must not add a toolbar button");
  const photoBox = await photo.boundingBox();
  assert(originalBox && photoBox && originalBox.x === photoBox.x && originalBox.width === photoBox.width, "Image must occupy the settings slot exactly");
  await page.locator(".custom-keyboard-shell").screenshot({ path: "/tmp/keyboard-image-fn@2x.png" });
  const png = Buffer.from(await page.evaluate(() => {
    const canvas = document.createElement("canvas"); canvas.width = 16; canvas.height = 16;
    const context = canvas.getContext("2d")!; context.fillStyle = "#3b82f6"; context.fillRect(0, 0, 16, 16);
    return canvas.toDataURL().split(",")[1];
  }), "base64");
  const choose = async () => {
    const chooser = page.waitForEvent("filechooser");
    await photo.click();
    await (await chooser).setFiles({ name: "phone-photo.png", mimeType: "image/png", buffer: png });
    await page.getByRole("dialog").getByRole("img").waitFor();
  };
  const keydowns = () => page.evaluate(() => (window as unknown as { fixture: { log: { type: string; key?: string; modifiers?: string[] }[] } }).fixture.log.filter(message => message.type === "keydown" && message.key === "v"));
  let release: (() => void) | undefined;
  let received = false;
  let fail = false;
  await page.route("**/api/clipboard/image", async route => {
    received = true;
    assert(route.request().headers()["content-type"] === "image/png", "Upload must be PNG");
    assert(route.request().postDataBuffer()?.subarray(1, 4).toString() === "PNG", "Upload must contain actual image bytes");
    if (fail) { await route.fulfill({ status: 503, body: "Desktop clipboard unavailable" }); return; }
    await new Promise<void>(resolve => { release = resolve; });
    await route.fulfill({ status: 204 });
  });
  await choose();
  await page.locator(".custom-keyboard-shell").screenshot({ path: "/tmp/keyboard-image-preview@2x.png" });
  await page.getByRole("button", { name: "Cancel image paste", exact: true }).click();
  assert(!received, "Cancelling preview must not upload");
  await choose();
  const before = (await keydowns()).length;
  await page.getByRole("button", { name: "Paste", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Sending image" }).waitFor();
  await page.waitForTimeout(100);
  assert(received && (await keydowns()).length === before, "Paste shortcut must wait for clipboard acknowledgement");
  release!();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  const sent = await keydowns();
  assert(sent.length === before + 1 && sent.at(-1)?.modifiers?.includes("ctrl"), "Successful upload must send Ctrl+V");
  assert(await page.evaluate(() => {
    const log = (window as unknown as { fixture: { log: { type: string; key?: string; modifiers?: string[] }[] } }).fixture.log;
    const last = log.at(-1);
    return last?.type === "keyup" && last.key === "Control" && last.modifiers?.length === 0;
  }), "Paste must release Ctrl immediately");
  fail = true;
  await choose();
  await page.getByRole("button", { name: "Paste", exact: true }).click();
  await page.getByRole("alert").filter({ hasText: "Desktop clipboard unavailable" }).waitFor();
  assert((await keydowns()).length === before + 1, "Failed upload must not paste old clipboard contents");
  await page.keyboard.press("Escape");
  await fn.click();
  assert(await settings.isVisible() && !await photo.isVisible(), "Disabling Fn must restore settings");
  await page.setViewportSize({ width: 320, height: 740 });
  await fn.click();
  const narrowBox = await photo.boundingBox();
  assert(narrowBox && narrowBox.x >= 0 && narrowBox.x + narrowBox.width <= 320, "Image button must fit a narrow phone");
  await fn.click();
  await page.setViewportSize({ width: 402, height: 874 });
  await page.unroute("**/api/clipboard/image");
  console.log("PASS: Fn image slot, photo selection, preview cancellation, upload acknowledgement, failure handling, modifier release and narrow layout");
}
