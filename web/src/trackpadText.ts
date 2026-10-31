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

  let start = 0;
  const commonLength = Math.min(before.length, after.length);
  while (start < commonLength && before[start] === after[start]) start++;

  let beforeEnd = before.length;
  let afterEnd = after.length;
  while (
    beforeEnd > start &&
    afterEnd > start &&
    before[beforeEnd - 1] === after[afterEnd - 1]
  ) {
    beforeEnd--;
    afterEnd--;
  }

  return {
    start,
    removed: before.slice(start, beforeEnd),
    inserted: after.slice(start, afterEnd),
  };
}

export function characterCount(value: string): number {
  return Array.from(value).length;
}
