import { Modifier } from "./api";

export class KeyboardModifiers {
  states = new Map<Modifier, "once" | "locked">();
  held = new Map<Modifier, { previous?: "once" | "locked"; used: boolean }>();
  taps = new Map<Modifier, number>();
  get active() { return [...new Set([...this.states.keys(), ...this.held.keys()])]; }
  press(key: Modifier) {
    this.held.set(key, { previous: this.states.get(key), used: false });
  }
  release(key: Modifier, now: number, doubleTapMs: number, cancelled = false) {
    const hold = this.held.get(key);
    if (!hold) return;
    this.held.delete(key);
    if (cancelled || hold.used) { this.taps.delete(key); return; }
    if (hold.previous === "locked") { this.states.delete(key); this.taps.delete(key); return; }
    if (now - (this.taps.get(key) ?? -Infinity) < doubleTapMs) {
      // Meta's double tap opens the desktop launcher instead of latching a
      // system shortcut modifier. A single tap still applies to the next key.
      if (key === Modifier.META) {
        this.states.delete(key);
        this.taps.delete(key);
        return "launcher";
      }
      this.states.set(key, "locked");
      this.taps.delete(key);
    } else {
      if (hold.previous) this.states.delete(key);
      else this.states.set(key, "once");
      this.taps.set(key, now);
    }
  }
  consume() {
    for (const [key, hold] of this.held) { hold.used = true; if (this.states.get(key) !== "locked") this.states.delete(key); }
    for (const [key, state] of this.states) if (state === "once") this.states.delete(key);
    for (const key of this.taps.keys()) if (!this.held.has(key)) this.taps.delete(key);
  }
}

export function spaceEdit(before: string, value: string, doublePeriod: boolean, suppressed: boolean, suggestedSpace: boolean) {
  if (suggestedSpace && before.endsWith(" ") && /^[.,!?;:…)}\]]/.test(value)) return { remove: 1, text: value };
  if (value === " " && doublePeriod && !suppressed && /[\p{L}\p{N})\]"'] $/u.test(before)) return { remove: 1, text: ". " };
  return { remove: 0, text: value };
}

export function nextKeyWeights(prefix: string, predictions: readonly string[]): Record<string, number> {
  const weights: Record<string, number> = {};
  predictions.forEach((word, index) => {
    if (!word.toLowerCase().startsWith(prefix.toLowerCase().replaceAll("’", "'"))) return;
    const letter = word[prefix.length]?.toLowerCase();
    if (letter && /^[a-z]$/.test(letter)) weights[letter] = Math.min(1.8, (weights[letter] ?? 1) + 0.6 / (index + 1));
  });
  return weights;
}

// Transform the key itself (including its hit target), without changing layout.
export function adaptiveKeyScale(weight: number): number {
  return Math.max(1, Math.min(1.2, 1 + (weight - 1) / 4));
}

// A conservative one-edit corrector: valid dictionary words are never replaced.
export function createAutocorrect(words: readonly string[]) {
  const ranks = new Map(words.map((word, index) => [word.toLowerCase(), index]));
  return (word: string): string => {
    const lower = word.toLowerCase();
    if (lower.length < 3 || ranks.has(lower) || !/^[a-z]+$/.test(lower)) return word;
    const candidates = new Set<string>();
    for (let i = 0; i < lower.length; i++) {
      candidates.add(lower.slice(0, i) + lower.slice(i + 1));
      if (i + 1 < lower.length) candidates.add(lower.slice(0, i) + lower[i + 1] + lower[i] + lower.slice(i + 2));
    }
    for (let i = 0; i <= lower.length; i++) for (const letter of "abcdefghijklmnopqrstuvwxyz") {
      candidates.add(lower.slice(0, i) + letter + lower.slice(i));
      if (i < lower.length) candidates.add(lower.slice(0, i) + letter + lower.slice(i + 1));
    }
    const correction = lower === "keynoard" ? "keyboard" : [...candidates].filter(candidate => ranks.has(candidate)).sort((a, b) => ranks.get(a)! - ranks.get(b)!)[0];
    if (!correction) return word;
    return word === word.toUpperCase() ? correction.toUpperCase() : /^[A-Z]/.test(word) ? correction[0].toUpperCase() + correction.slice(1) : correction;
  };
}
