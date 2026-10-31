import { STARTER_RECENTS } from "./keyboardData";
import { parseSettings } from "./trackpadSettings";

function readPreference(key: string): unknown {
  try {
    const saved = localStorage.getItem(key);
    return saved === null ? undefined : JSON.parse(saved) as unknown;
  } catch {
    return undefined;
  }
}

export function savePreference(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Storage may be disabled or full. Settings still work for this session.
  }
}

export function loadSettings() {
  return parseSettings(readPreference("ottoman-trackpad-settings"));
}

export function loadRecentEmoji(): string[] {
  const saved = readPreference("ottoman-recent-emoji");
  if (!Array.isArray(saved)) return [...STARTER_RECENTS];
  // Validate a bounded emoji-only sequence without eagerly importing the full
  // Unicode catalogue. Allow joiners, flags, keycaps, selectors, and skin tones.
  const emojiSequence = /^(?:\p{Extended_Pictographic}|\p{Regional_Indicator}|[0-9#*]\uFE0F?\u20E3)(?:\p{Extended_Pictographic}|\p{Regional_Indicator}|\p{Emoji_Modifier}|(?:\u200D|\uFE0F)|[\u{E0020}-\u{E007F}])*$/u;
  return [...new Set(saved.filter((value): value is string =>
    typeof value === "string" && value.length <= 64 && emojiSequence.test(value),
  ))].slice(0, 12);
}
