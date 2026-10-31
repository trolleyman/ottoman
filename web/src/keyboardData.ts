export interface KeyDefinition {
  key: string;
  alt?: string;
  grow?: number;
}

export type KeyboardLayout = "en-GB" | "en-US";

export function resolveKeyboardLayout(choice: KeyboardLayout | "auto", locale: string): KeyboardLayout {
  return choice === "auto" ? (/^en-US(?:-|$)/i.test(locale) ? "en-US" : "en-GB") : choice;
}

export function keyboardRows(layout: KeyboardLayout) {
  const shiftedNumbers = layout === "en-GB" ? '!"£$%^&*()' : '!@#$%^&*()';
  const numberRow: KeyDefinition[] = Array.from("1234567890", (key, index) => ({
    key, alt: Array.from(shiftedNumbers)[index],
  }));
  // A compact mobile layout: hyphen on the left; question mark and apostrophe
  // next to backspace. Symbol mode is the same grid with its grey hints promoted.
  const letterRows: KeyDefinition[][] = [
    Array.from("qwertyuiop", (key, index) => ({ key, alt: Array.from(shiftedNumbers)[index] })),
    Array.from("asdfghjkl", (key, index) => ({ key, alt: Array.from("-/:;[]=+#")[index] })),
    Array.from("zxcvbnm", (key, index) => ({ key, alt: Array.from("\\|,.@?'")[index] })),
  ];
  return {
    numberRow,
    letterRows,
    symbolRows: letterRows.map((row) => row.map(({ alt }) => ({ key: alt! }))),
  };
}

export const SYMBOLS = [
  ["£", "pound sterling currency british gbp"], ["€", "euro currency"],
  ["$", "dollar currency usd"], ["¥", "yen yuan currency"], ["¢", "cent currency"],
  ["©", "copyright"], ["®", "registered trademark"], ["™", "trademark"],
  ["°", "degree temperature angle"], ["±", "plus minus"], ["×", "multiply times"],
  ["÷", "divide division"], ["≈", "approximately equal"], ["≠", "not equal"],
  ["≤", "less than or equal"], ["≥", "greater than or equal"], ["∞", "infinity"],
  ["√", "square root"], ["π", "pi"], ["µ", "micro micron"], ["Ω", "omega ohm"],
  ["→", "right arrow"], ["←", "left arrow"], ["↑", "up arrow"], ["↓", "down arrow"],
  ["•", "bullet dot"], ["…", "ellipsis dots"], ["§", "section"], ["¶", "paragraph"],
  ["†", "dagger"], ["‡", "double dagger"], ["✓", "check tick"], ["—", "em dash"],
  ["–", "en dash"], ["№", "number numero"], ["@", "at sign"], ["#", "hash pound number"],
] as const;

export const STARTER_RECENTS = ["😂", "❤️", "👍", "🙏", "🔥", "🎉"];

