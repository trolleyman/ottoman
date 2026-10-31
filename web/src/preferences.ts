import { EMOJIS, STARTER_RECENTS } from "./keyboardData";
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
  const known = new Set<string>(EMOJIS.map(([emoji]) => emoji));
  return [...new Set(saved.filter((value): value is string =>
    typeof value === "string" && known.has(value),
  ))].slice(0, 12);
}
