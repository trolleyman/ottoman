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

  const oldChars = Array.from(before);
  const newChars = Array.from(after);
  let prefix = 0;
  while (prefix < Math.min(oldChars.length, newChars.length) && oldChars[prefix] === newChars[prefix]) prefix++;
  let oldEnd = oldChars.length;
  let newEnd = newChars.length;
  while (oldEnd > prefix && newEnd > prefix && oldChars[oldEnd - 1] === newChars[newEnd - 1]) { oldEnd--; newEnd--; }
  return {
    start: oldChars.slice(0, prefix).join("").length,
    removed: oldChars.slice(prefix, oldEnd).join(""),
    inserted: newChars.slice(prefix, newEnd).join(""),
  };
}

export function characterCount(value: string): number {
  return Array.from(value).length;
}
