import type { Browser } from 'playwright';
import type { TrackpadMessageTextContext, TrackpadMessageTextEdit } from '../../src/api';
import { readFileSync } from 'node:fs';

export async function installTestFont(page: import('playwright').Page) {
  // Optional explicit font for headless WebKit environments without system fonts.
  if (!process.env.TEXT_TEST_FONT) return;
  const data = readFileSync(process.env.TEXT_TEST_FONT).toString('base64');
  await page.addStyleTag({ content: `@font-face { font-family: CaretTest; src: url(data:font/ttf;base64,${data}); } * { font-family: CaretTest !important; }` });
  await page.evaluate(() => document.fonts.ready);
}

export async function checkAddressBar(browser: Browser, base: string) {
  const page = await browser.newPage({ viewport: { width: 402, height: 874 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  let state: TrackpadMessageTextContext = {
    type: 'textcontext', fieldId: 'omnibox', revision: 1, value: 'g', selectionStart: 1, selectionEnd: 1,
    windowStart: 0, available: true, editable: true, multiline: false, protected: false, password: false,
  };
  let notify: ((next: TrackpadMessageTextContext) => void) | undefined;
  const applied: string[] = [];
  let lateCompletion = true;
  await page.addInitScript(() => localStorage.setItem('ottoman-trackpad-settings', JSON.stringify({
    mirrorFocusedText: true, customKeyboard: true,
  })));
  await page.routeWebSocket('**/api/trackpad', socket => {
    notify = next => { state = next; socket.send(JSON.stringify(state)); };
    socket.send(JSON.stringify(state));
    socket.onMessage(raw => {
      const message = JSON.parse(String(raw)) as TrackpadMessageTextEdit | { type: 'textwatch' };
      if (message.type === 'textwatch') { socket.send(JSON.stringify(state)); return; }
      if (message.type !== 'textedit') return;
      setTimeout(() => {
        if (lateCompletion) {
          // The suggestion appeared after the last poll, before the first edit.
          lateCompletion = false;
          state = { ...state, value: 'google.com', selectionStart: 1, selectionEnd: 10, revision: 2 };
          socket.send(JSON.stringify({ ...state, requestId: message.requestId, rejected: true }));
          return;
        }
        if (message.baseRevision !== state.revision) {
          errors.push('Queued edit used a stale revision');
          socket.send(JSON.stringify({ ...state, requestId: message.requestId, rejected: true }));
          return;
        }
        const chars = Array.from(state.value);
        const prefix = chars.slice(0, message.start).join('') + message.text + chars.slice(message.end).join('');
        applied.push(message.text);
        state = { ...state, value: prefix + '.example', selectionStart: Array.from(prefix).length, selectionEnd: Array.from(prefix).length + 8, revision: state.revision + 1 };
        // Polling before acknowledgment must not replace the optimistic burst.
        socket.send(JSON.stringify(state));
        socket.send(JSON.stringify({ ...state, requestId: message.requestId }));
      }, 120);
    });
  });
  try {
    await page.goto(base + '/remote');
    await installTestFont(page);
    await page.locator('[data-text-preview] [data-caret-text]').filter({ hasText: 'g' }).waitFor();
    await page.getByLabel(/Trackpad\. Tap to click/).focus();
    for (const letter of 'oing') await page.locator(`[data-letter="${letter}"]`).click();
    await page.waitForFunction(() => document.querySelector('[data-text-preview] [data-caret-text]')?.textContent === 'going.example\u200b');
    if (applied.join('') !== 'oing' || state.selectionStart !== 5) throw Error(`Address-bar typing lost or duplicated characters: ${state.value}`);
    console.log('PASS: queued going survives a stale revision and changing inline completions');

    for (const value of ['I think therefore i am', '', 'A wrapped line of text that fills more than one line in the preview', 'First line\n', 'Emoji 😀 at the end']) {
      notify!({ ...state, value, selectionStart: Array.from(value).length, selectionEnd: Array.from(value).length, revision: state.revision + 1, multiline: true });
      await page.waitForFunction(text => document.querySelector('[data-text-preview] [data-caret-text]')?.textContent === text + '\u200b', value);
      const delta = await page.locator('[data-preview-caret]').evaluate(caret => {
        const text = document.querySelector('[data-text-preview] [data-caret-text]')!;
        const end = Array.from(text.getClientRects()).at(-1)!;
        const actual = caret.getBoundingClientRect();
        return { x: Math.abs(actual.left - end.right), y: Math.abs(actual.top - end.top), height: actual.height };
      });
      if (delta.x > 0.1 || delta.y > 0.1 || delta.height <= 0) throw Error(`End caret drifted for ${JSON.stringify(value)}: ${JSON.stringify(delta)}`);
    }
    // Changing font metrics must not leave the caret at its old measured x.
    await page.locator('[data-text-preview]').evaluate(element => { (element as HTMLElement).style.letterSpacing = '1.3px'; });
    const aligned = await page.locator('[data-preview-caret]').evaluate(caret => {
      const end = Array.from(document.querySelector('[data-text-preview] [data-caret-text]')!.getClientRects()).at(-1)!;
      return Math.abs(caret.getBoundingClientRect().left - end.right) < 0.1;
    });
    if (!aligned) throw Error('End caret did not follow changed text metrics');
    for (const value of ['مرحبا', 'hello مرحبا', 'مرحبا hello']) {
      notify!({ ...state, value, selectionStart: Array.from(value).length, selectionEnd: Array.from(value).length, revision: state.revision + 1 });
      await page.waitForFunction(text => document.querySelector('[data-text-preview] [data-caret-text]')?.textContent === text + '\u200b', value);
      const logicalEnd = await page.locator('[data-preview-caret]').evaluate(caret => {
        const node = document.querySelector('[data-text-preview] [data-caret-text]')!.firstChild!;
        const range = document.createRange();
        range.setStart(node, node.textContent!.length - 1); range.collapse(true);
        return Math.abs(caret.getBoundingClientRect().left - range.getBoundingClientRect().left) < 1;
      });
      if (!logicalEnd) throw Error('Bidirectional text lost its logical caret endpoint');
    }
    console.log('PASS: end caret follows real text layout, wrapping, trailing newlines, and changed glyph spacing');
    if (errors.length) throw Error(errors.join('\n'));
  } finally { await page.close(); }
}
