import type { Browser } from 'playwright';
import type { TrackpadMessage, TrackpadMessageTextContext } from '../../src/api';

export async function checkDelayedTypingEcho(browser: Browser, base: string) {
  const page = await browser.newPage({ viewport: { width: 402, height: 874 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
  let state: TrackpadMessageTextContext = {
    type: 'textcontext', fieldId: 'rich-editor', revision: 1, value: '', selectionStart: 0, selectionEnd: 0,
    windowStart: 0, available: true, editable: false, multiline: true, protected: false, password: false,
  };
  let desktop = '';
  let echo: (() => void) | undefined;
  await page.addInitScript(() => localStorage.setItem('ottoman-trackpad-settings', JSON.stringify({
    mirrorFocusedText: true, customKeyboard: true, swipeTyping: true,
  })));
  await page.routeWebSocket('**/api/trackpad', socket => {
    echo = () => {
      state = { ...state, value: desktop, selectionStart: Array.from(desktop).length, selectionEnd: Array.from(desktop).length, revision: state.revision + 1 };
      socket.send(JSON.stringify(state));
    };
    socket.send(JSON.stringify(state));
    socket.onMessage(raw => {
      const message = JSON.parse(String(raw)) as TrackpadMessage;
      if (message.type === 'textwatch') socket.send(JSON.stringify(state));
      if (message.type === 'text') desktop += message.text;
      if (message.type === 'keydown' && message.key === 'Enter') desktop += '\n';
    });
  });
  try {
    await page.goto(base + '/remote');
    await page.getByRole('status', { name: 'Desktop text: connected', exact: true }).waitFor();
    await page.getByLabel(/Trackpad\. Tap to click/).focus();
    await page.locator('[data-swipe-ready="true"]').waitFor();
    const phrase = 'also\ncharacters\n\nlast line';
    let expected = '';
    for (const character of phrase) {
      const key = character === '\n' ? page.getByRole('button', { name: 'Enter', exact: true })
        : character === ' ' ? page.getByRole('button', { name: 'Space. Hold and drag to move the cursor', exact: true })
        : page.locator(`[data-letter="${character}"]`);
      const box = await key.boundingBox();
      if (!box) throw Error(`Missing key ${JSON.stringify(character)}`);
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      await page.mouse.down();
      // The preceding letter's desktop echo arrives while the next tap is held.
      echo!();
      await page.waitForFunction(value => document.querySelector('[data-text-preview] [data-caret-text]')?.textContent === value + '\u200b', expected);
      if (expected.length % 2) await page.mouse.move(box.x + box.width / 2 + 1, box.y + box.height / 2);
      await page.mouse.up();
      expected += character;
      // Wait for WebSocket delivery without publishing this key's desktop echo.
      for (let attempt = 0; desktop !== expected && attempt < 50; attempt++) await page.waitForTimeout(10);
      if (desktop !== expected) throw Error(`Typing lost a character or newline: expected ${JSON.stringify(expected)}, received ${JSON.stringify(desktop)}`);
    }
    echo!();
    await page.waitForFunction(value => document.querySelector('[data-text-preview] [data-caret-text]')?.textContent === value + '\u200b', phrase);
    const start = await page.locator('[data-letter="h"]').boundingBox();
    const finish = await page.locator('[data-letter="e"]').boundingBox();
    if (!start || !finish) throw Error('Missing swipe keys');
    await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
    await page.mouse.down();
    await page.mouse.move(finish.x + finish.width / 2, finish.y + finish.height / 2, { steps: 5 });
    const external = desktop + '\nexternal';
    desktop = external;
    echo!();
    await page.waitForFunction(value => document.querySelector('[data-text-preview] [data-caret-text]')?.textContent === value + '\u200b', external);
    await page.mouse.up();
    await page.waitForTimeout(50);
    if (desktop !== external) throw Error('An invalidated glide became a stray tap');
    console.log('PASS: delayed desktop echoes during held taps preserve also, characters, spaces and consecutive newlines');
  } finally { await page.close(); }
}
