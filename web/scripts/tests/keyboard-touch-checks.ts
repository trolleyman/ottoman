import type { Page } from 'playwright';

export async function checkKeyboardTouchDefaults(page: Page) {
  const blocked = await page.locator('.custom-keyboard-shell').evaluate(keyboard => {
    const letter = keyboard.querySelector('[data-letter="i"]')!;
    const targets = [
      letter, letter.querySelector('.relative')!,
      keyboard.querySelector('[aria-label="8, hold for *"]')!,
      keyboard.querySelector('[aria-label="Backspace"]')!,
      keyboard.querySelector('[aria-label="Space. Hold and drag to move the cursor"]')!,
      keyboard.querySelector('[aria-label="Keyboard controls"] button:nth-child(2)')!,
    ];
    return targets.every(target => {
      const touch = new Event('touchstart', { bubbles: true, cancelable: true });
      const menu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
      target.dispatchEvent(touch);
      target.dispatchEvent(menu);
      return touch.defaultPrevented && menu.defaultPrevented;
    });
  });
  if (!blocked) throw Error('Keys and their glyphs must suppress native iOS hold/magnifier gestures');

  await page.getByRole('button', { name: 'Emoji', exact: true }).click();
  const pickerScrolls = await page.getByRole('region', { name: 'All emoji', exact: true }).getByRole('button').first().evaluate(button => {
    const touch = new Event('touchstart', { bubbles: true, cancelable: true });
    button.dispatchEvent(touch);
    return !touch.defaultPrevented;
  });
  if (!pickerScrolls) throw Error('Emoji buttons must retain native touch scrolling');
  await page.getByRole('button', { name: 'Letters', exact: true }).click();

  await page.getByRole('button', { name: 'Keyboard settings', exact: true }).click();
  const settingsScrolls = await page.getByRole('button', { name: 'Back to keyboard', exact: true }).evaluate(button => {
    const touch = new Event('touchstart', { bubbles: true, cancelable: true });
    button.dispatchEvent(touch);
    return !touch.defaultPrevented;
  });
  const inputWorks = await page.getByLabel('Symbol hold milliseconds', { exact: true }).evaluate(input => {
    const touch = new Event('touchstart', { bubbles: true, cancelable: true });
    input.dispatchEvent(touch);
    return !touch.defaultPrevented;
  });
  if (!settingsScrolls || !inputWorks) throw Error('Keyboard settings must retain native scrolling and editable controls');
  await page.getByRole('button', { name: 'Back to keyboard', exact: true }).click();
  console.log('PASS: native key hold gestures are blocked while picker pans and settings input remain available');
}
