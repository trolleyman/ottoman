export interface TextEdit {
  start: number;
  removed: string;
  inserted: string;
}

// Find the smallest replacement that turns `before` into `after`. Keeping a
// real input value on the phone lets iOS perform autocorrection, double-space
// punctuation and composition; this edit is what must be mirrored remotely.
export function diffText(before: string, after: string): TextEdit | null {
  if (before === after) return null;

  // Compare whole code points: two different emoji can share one UTF-16
  // surrogate. Sending only the changed half would inject invalid text.
  const beforeCharacters = Array.from(before);
  const afterCharacters = Array.from(after);
  let start = 0;
  const commonLength = Math.min(beforeCharacters.length, afterCharacters.length);
  while (start < commonLength && beforeCharacters[start] === afterCharacters[start]) start++;

  let beforeEnd = beforeCharacters.length;
  let afterEnd = afterCharacters.length;
  while (
    beforeEnd > start &&
    afterEnd > start &&
    beforeCharacters[beforeEnd - 1] === afterCharacters[afterEnd - 1]
  ) {
    beforeEnd--;
    afterEnd--;
  }

  return {
    // Textareas use UTF-16 offsets, so keep the public start index in those units.
    start: beforeCharacters.slice(0, start).join("").length,
    removed: beforeCharacters.slice(start, beforeEnd).join(""),
    inserted: afterCharacters.slice(start, afterEnd).join(""),
  };
}

export function characterCount(value: string): number {
  return Array.from(value).length;
}
