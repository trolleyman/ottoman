// Isolated preview server: ephemeral localhost port, no desktop connection.
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../../../', import.meta.url));
const require = createRequire(resolve(root, 'web/package.json'));
const { createServer } = await import(require.resolve('vite'));
const { chromium } = require('playwright');
const out = resolve(process.argv[2] || '/tmp/ottoman-remote-design-v2');
await mkdir(out, { recursive: true });
const server = await createServer({
  root, cacheDir: resolve(root, 'web/node_modules/.vite/remote-design'), configFile: resolve(root, 'web/vite.config.ts'), publicDir: resolve(root, 'web/public'),
  resolve: { alias: { react: resolve(root, 'web/node_modules/react'), 'react-dom': resolve(root, 'web/node_modules/react-dom') } },
  optimizeDeps: { entries: ['docs/design/pc-remote/v2.html'] },
  server: { host: '127.0.0.1', port: 0 },
});
let browser;
try {
  await server.listen(0);
  const address = server.httpServer.address();
  const base = `http://127.0.0.1:${address.port}/docs/design/pc-remote/v2.html`;
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH });
  for (const view of ['layouts', 'details']) {
    const page = await browser.newPage({ viewport: { width: 1290, height: 980 }, deviceScaleFactor: 2 });
    page.on('pageerror', error => { throw error; });
    await page.goto(`${base}?view=${view}`);
    await page.locator('.custom-keyboard-shell').first().waitFor();
    if (view === 'details') await page.getByRole('region', { name: 'All emoji' }).waitFor();
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: `${out}/${view}@2x.png`, fullPage: true });
    await page.close();
  }
  const page = await browser.newPage({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
  await page.goto(`${base}?view=phone`);
  await page.locator('.custom-keyboard-shell').waitFor();
  const keyboard = page.locator('.v2-keyboard');
  const before = await keyboard.boundingBox();
  await page.getByRole('button', { name: 'Emoji', exact: true }).click();
  await page.getByRole('region', { name: 'All emoji' }).waitFor();
  const after = await keyboard.boundingBox();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth);
  if (overflow || before.width !== after.width) throw Error('Emoji changes the viewport width');
  await page.screenshot({ path: `${out}/emoji-phone@2x.png` });
  await page.goto(`${base}?view=phone`);
  await page.locator('.custom-keyboard-shell').waitFor();
  const pad = await page.locator('.v2-pad').boundingBox();
  await page.mouse.move(pad.x + 15, pad.y + 10);
  await page.mouse.down();
  await page.waitForTimeout(560);
  if (!await page.getByRole('status').isVisible()) throw Error('Stationary hold did not fire');
  await page.mouse.up();
  await page.getByRole('status').click();
  await page.mouse.move(pad.x + 15, pad.y + 10);
  await page.mouse.down();
  await page.mouse.move(pad.x + 45, pad.y + 10);
  await page.waitForTimeout(560);
  await page.mouse.up();
  if (await page.getByRole('status').count()) throw Error('Moved hold must not right-click');
  console.log('Hold preview: stationary gesture fires; movement cancels.');
  await page.setViewportSize({ width: 390, height: 610 });
  await page.goto(`${base}?view=gestures`);
  await page.getByLabel('Hold duration', { exact: true }).waitFor();
  await page.screenshot({ path: `${out}/gestures@2x.png` });
  console.log('390px: emoji stays within viewport; keyboard width is unchanged.');
  console.log(out);
} finally { await browser?.close(); await server.close(); }
