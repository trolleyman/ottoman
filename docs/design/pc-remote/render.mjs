// Render review boards at 2x without starting a server or contacting a desktop.
import { createRequire } from 'node:module';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const require = createRequire(new URL('../../../web/package.json', import.meta.url));
const { chromium } = require('playwright');
const output = resolve(process.argv[2] || '/tmp/ottoman-remote-design');
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH });
try {
  for (const [view, width] of [['options', 900], ['states', 1200], ['desktop', 1200], ['clipboard', 900]]) {
    const page = await browser.newPage({ viewport: { width, height: 1000 }, deviceScaleFactor: 2 });
    await page.goto(`file://${fileURLToPath(new URL('./index.html', import.meta.url))}?view=${view}`);
    await page.screenshot({ path: `${output}/${view}@2x.png`, fullPage: true });
    await page.close();
  }
} finally { await browser.close(); }
console.log(output);
