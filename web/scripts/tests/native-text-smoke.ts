// Run from web after `bun run build`, with a project-local Playwright Chromium.
import { chromium, webkit } from 'playwright';
import { checkAddressBar, installTestFont } from './address-bar-checks';
import { spawn, spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createServer } from 'node:net';

const root = resolve(import.meta.dir, '../../..');
const work = mkdtempSync(join(tmpdir(), 'ottoman-native-smoke-'));
const bin = join(work, 'ottoman');
const build = spawnSync('go', ['build', '-o', bin, './cmd/ottoman'], { cwd: root, stdio: 'inherit' });
if (build.status !== 0) throw Error('Simulator build failed');

const port = await new Promise<number>((resolvePort) => {
  const listener = createServer();
  listener.listen(0, '127.0.0.1', () => {
    const address = listener.address() as { port: number };
    listener.close(() => resolvePort(address.port));
  });
});
writeFileSync(join(work, 'controller.toml'), `
[controller]
listen_address = "127.0.0.1:${port}"
auth_token = "simulated"
[controller.agent]
url = "http://127.0.0.1:17294"
`);
writeFileSync(join(work, 'agent.toml'), `
[agent]
auth_token = "simulated"
listen_address = "127.0.0.1:17294"
`);
const server = spawn(bin, [
  '--config', join(work, 'controller.toml'), 'controller', 'simulate',
  '--agent-config', join(work, 'agent.toml'), '--start-online',
], { cwd: root, stdio: 'ignore' });
let browser;
try {
  const base = `http://127.0.0.1:${port}`;
  let ready = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { ready = (await fetch(`${base}/health`)).ok; }
    catch { /* Wait for the isolated simulator to bind. */ }
    if (ready) break;
    await Bun.sleep(100);
  }
  if (!ready) throw Error('Simulator did not become ready');
  browser = await (process.env.TEXT_TEST_BROWSER === "webkit" ? webkit : chromium).launch();
  const page = await browser.newPage({ viewport: { width: 402, height: 874 }, deviceScaleFactor: 2 });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.addInitScript(() => localStorage.setItem('ottoman-trackpad-settings', JSON.stringify({
    mirrorFocusedText: true, customKeyboard: false,
  })));
  await page.goto(base);
  await installTestFont(page);
  const input = page.getByLabel('Type on the remote computer with the device keyboard');
  await input.waitFor({ state: 'attached' });
  console.log("Checking initial native field");
  await page.waitForFunction(() => document.querySelector('textarea')?.value === 'Ottoman simulated text');
  await page.getByLabel(/Trackpad\. Tap to click/).focus();
  await page.waitForFunction(() => document.activeElement?.tagName === "TEXTAREA");
  console.log("Checking native typing");
  await input.press('End');
  await input.pressSequentially(' xyz', { delay: 5 });
  await page.waitForFunction(() => document.querySelector('textarea')?.value === 'Ottoman simulated text xyz');
  console.log("Checking Unicode selection replacement");
  // Use actual selection events; a synthetic `select` DOM event does not
  // exercise React's document selectionchange/keyup selection plugin.
  const previewText = page.locator('[data-text-preview] [data-caret-text]');
  const glyphPositions = () => previewText.evaluate((element) => {
    const node = element.firstChild!;
    const origin = element.getBoundingClientRect();
    return Array.from({ length: node.textContent!.length - 1 }, (_, index) => {
      const range = document.createRange();
      range.setStart(node, index); range.setEnd(node, index + 1);
      const rect = range.getBoundingClientRect();
      return [rect.left - origin.left, rect.top - origin.top, rect.width];
    });
  });
  const beforeCaretMove = await glyphPositions();
  for (let index = 0; index < 9; index++) await input.press('ArrowLeft');
  if (JSON.stringify(await glyphPositions()) !== JSON.stringify(beforeCaretMove)) throw Error('Moving the preview caret shifted the text');
  const caretAligned = await page.locator('[data-preview-caret]').evaluate((caret) => {
    const node = document.querySelector('[data-text-preview] [data-caret-text]')!.firstChild!;
    const range = document.createRange(); range.setStart(node, 17); range.collapse(true);
    const expected = range.getBoundingClientRect(), actual = caret.getBoundingClientRect();
    return getComputedStyle(caret).position === 'absolute' && Math.abs(expected.left - actual.left) < 1 && Math.abs(expected.top - actual.top) < 1;
  });
  if (!caretAligned) throw Error('Preview caret is not aligned with the native text position');
  for (let index = 0; index < 9; index++) await input.press('Shift+ArrowLeft');
  await page.waitForFunction(() => {
    const input = document.querySelector('textarea');
    return input?.selectionStart === 8 && input.selectionEnd === 17;
  });
  if (await page.locator('[data-preview-caret]').count()) throw Error('A selected range must not show an extra blinking caret');
  await page.keyboard.insertText('😀');
  await page.waitForFunction(() => document.querySelector('textarea')?.value === 'Ottoman 😀 text xyz');
  console.log("Checking live preview during spacebar navigation");
  await page.getByRole('button', { name: 'Open keyboard settings' }).click();
  await page.getByRole('button', { name: 'Ottoman', exact: true }).click();
  await page.getByRole('button', { name: 'Back to keyboard', exact: true }).click();
  const space = page.getByRole('button', { name: 'Space. Hold and drag to move the cursor' });
  await space.scrollIntoViewIfNeeded();
  const spaceBox = await space.boundingBox();
  if (!spaceBox) throw Error('Missing spacebar');
  // Watch removals as well as the final state: a desktop update can restore the
  // preview before a visibility assertion notices the flash.
  await page.evaluate(() => {
    const preview = document.querySelector('[data-text-preview]')!;
    const observer = new MutationObserver((records) => {
      if (records.some(record => Array.from(record.removedNodes).some(node => node === preview || node.contains(preview)))) {
        document.documentElement.dataset.previewRemoved = 'true';
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  });
  const x = spaceBox.x + spaceBox.width / 2, y = spaceBox.y + spaceBox.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(500);
  for (const offset of [-24, -48, -24, 0]) {
    await page.mouse.move(x + offset, y);
    await page.waitForTimeout(80);
    if (!await page.locator('[data-text-preview]').isVisible()) throw Error('Navigation hid the live preview');
  }
  await page.mouse.up();
  if (await page.evaluate(() => document.documentElement.dataset.previewRemoved)) throw Error('Live preview flickered during spacebar navigation');
  console.log("Checking disable clearing");
  await page.getByRole('button', { name: 'Keyboard settings', exact: true }).click();
  await page.getByRole('switch', { name: 'Mirror focused text' }).click();
  await page.getByRole('button', { name: 'Device', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('textarea')?.value === '');
  if (errors.length) throw Error(errors.join('\n'));
  await checkAddressBar(browser, base);
  console.log('PASS: native typing, queued edits, Unicode selection replacement, stable caret geometry, spacebar preview stability, and disable clearing');
} finally {
  await browser?.close();
  server.kill('SIGKILL');
  rmSync(work, { recursive: true, force: true });
}
