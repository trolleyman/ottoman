// Run from web after `bun run build`, with a project-local Playwright Chromium.
import { chromium } from 'playwright';
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
  browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 402, height: 874 }, deviceScaleFactor: 2 });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.addInitScript(() => localStorage.setItem('ottoman-trackpad-settings', JSON.stringify({
    mirrorFocusedText: true, customKeyboard: false,
  })));
  await page.goto(base);
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
  for (let index = 0; index < 9; index++) await input.press('ArrowLeft');
  for (let index = 0; index < 9; index++) await input.press('Shift+ArrowLeft');
  await page.waitForFunction(() => {
    const input = document.querySelector('textarea');
    return input?.selectionStart === 8 && input.selectionEnd === 17;
  });
  await page.keyboard.insertText('😀');
  await page.waitForFunction(() => document.querySelector('textarea')?.value === 'Ottoman 😀 text xyz');
  console.log("Checking disable clearing");
  await page.getByRole('button', { name: 'Open keyboard settings' }).click();
  await page.getByRole('switch', { name: 'Mirror focused text' }).click();
  await page.waitForFunction(() => document.querySelector('textarea')?.value === '');
  if (errors.length) throw Error(errors.join('\n'));
  console.log('PASS: native typing, queued edits, Unicode selection replacement, and disable clearing');
} finally {
  await browser?.close();
  server.kill('SIGKILL');
  rmSync(work, { recursive: true, force: true });
}
