// From web/: PLAYWRIGHT_BROWSERS_PATH="$PWD/.playwright" bun scripts/generate-app-icons.ts
// Keep the existing logo unchanged; installed icons get the app's opaque background.
import { chromium } from 'playwright';
import { readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const publicDir = resolve(import.meta.dir, '../public');
const logo = 'data:image/png;base64,' + readFileSync(join(publicDir, 'ottoman_logo.png')).toString('base64');
const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  for (const [name, size] of [
    ['apple-touch-icon-dark.png', 180],
    ['app-icon-192.png', 192],
    ['app-icon-512.png', 512],
  ] as const) {
    const png = await page.evaluate(async ({ logo, size }) => {
      const image = new Image();
      image.src = logo;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = canvas.height = size;
      const context = canvas.getContext('2d')!;
      context.fillStyle = '#09090b';
      context.fillRect(0, 0, size, size);
      const inset = Math.round(size * 0.06);
      context.drawImage(image, inset, inset, size - inset * 2, size - inset * 2);
      const pixels = context.getImageData(0, 0, size, size).data;
      for (let offset = 3; offset < pixels.length; offset += 4) {
        if (pixels[offset] !== 255) throw Error('Installed icon must be fully opaque');
      }
      return canvas.toDataURL('image/png').split(',')[1];
    }, { logo, size });
    writeFileSync(join(publicDir, name), Buffer.from(png, 'base64'));
    console.log(`Exported ${name}: ${size} × ${size}, opaque #09090b background`);
  }
} finally { await browser.close(); }
