import type { Page } from "playwright";

export async function checkArrowRepeat(page: Page, reset: (text: string, cursor?: number) => Promise<void>) {
  const assert = (condition: unknown, message: string) => { if (!condition) throw Error(message); };
  const position = () => page.evaluate<number>("window.fixture.context.selectionStart");
  const alphabet = "abcdefghijklmnopqrstuvwxyz";
  const lines = Array.from({ length: 12 }, () => "abcdefghij").join("\n");
  for (const direction of ["right", "left", "down", "up"]) {
    const value = direction === "right" || direction === "left" ? alphabet : lines;
    const start = direction === "left" || direction === "up" ? value.length : 0;
    await reset(value, start);
    const arrow = page.getByRole("button", { name: `Arrow ${direction}`, exact: true });
    const box = (await arrow.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    assert(await position() !== start, `Arrow ${direction} must move immediately on press`);
    await page.waitForTimeout(680);
    await page.mouse.up();
    const end = await position();
    assert(Math.abs(end - start) >= (direction === "up" || direction === "down" ? 33 : 4), `Holding Arrow ${direction} must repeat movement`);
    await page.waitForTimeout(160);
    assert(await position() === end, `Arrow ${direction} must stop repeating on release`);
  }
  await reset(alphabet, 0);
  const right = page.getByRole("button", { name: "Arrow right", exact: true });
  const box = (await right.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2); await page.mouse.down();
  await right.dispatchEvent("pointercancel", { pointerId: 1, pointerType: "mouse" });
  await page.mouse.up(); await page.waitForTimeout(500);
  assert(await position() === 1, "Cancelling an arrow hold must stop its repeat timer");
}
