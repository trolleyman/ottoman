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
  // British number shifts live on the number row, freeing Q–P for other
  // symbols. Keep backtick/pipe/backslash left and hash/tilde on the right.
  // Empty hints deliberately leave an unused cell in the matching symbol grid.
  const hints: (string | undefined)[][] = layout === "en-GB" ? [
    ["`", "€", undefined, "<", ">", "-", "=", "_", "#", "~"],
    [":", ";", "[", "]", "{", "}", "@", "+", undefined],
    ["|", "\\", ",", ".", "?", "'", "/"],
  ] : [
    Array.from(shiftedNumbers),
    Array.from("-/:;[]=+#"),
    Array.from("\\|,`@?'"),
  ];
  const letterRows: KeyDefinition[][] = ["qwertyuiop", "asdfghjkl", "zxcvbnm"].map((row, index) =>
    Array.from(row, (key, column) => ({ key, alt: hints[index][column] })),
  );
  return {
    numberRow,
    letterRows,
    symbolRows: letterRows.map((row) => row.map(({ alt }) => ({ key: alt ?? "" }))),
  };
}

export const STARTER_RECENTS = ["😂", "❤️", "👍", "🙏", "🔥", "🎉"];

