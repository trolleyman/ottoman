export const TERMINAL_FONTS = {
  iosevka: { label: "Iosevka Term Nerd Font Mono", family: '"Iosevka Term Nerd Font Mono", monospace' },
  "fira-code": { label: "Fira Code", family: '"Fira Code", "Iosevka Term Nerd Font Mono", monospace' },
  consolas: { label: "Consolas (if installed)", family: 'Consolas, "Iosevka Term Nerd Font Mono", monospace' },
} as const;

export interface KeyboardSettings {
  terminalScrollback: boolean;
  terminalFont: keyof typeof TERMINAL_FONTS;
  autocorrect: boolean;
  adaptiveKeys: boolean;
  debugKeySizes: boolean;
  previewNavigation: boolean;
  spaceCursorSensitivity: number;
  interactivePreview: boolean;
  mirrorFocusedText: boolean;
  keyboardLayout: "en-GB" | "en-US" | "auto";
  keyboardTheme: "dark" | "light" | "system";
  tabKey: boolean;
  emojiKey: boolean;
  periodKey: boolean;
  commaKey: boolean;
  swipeTyping: boolean;
  suggestionsAboveControls: boolean;
  numberRow: boolean;
  tallerKeys: boolean;
  symbolHints: boolean;
  doubleSpacePeriod: boolean;
  haptics: boolean;
  longPressDelayMs: number;
  spaceHoldDelayMs: number;
  backspaceRepeatDelayMs: number;
  backspaceRepeatIntervalMs: number;
  shiftDoubleTapMs: number;
}

export interface TrackpadSettings extends KeyboardSettings {
  cursorSensitivity: number;
  cursorFriction: number;
  scrollSensitivity: number;
  scrollFriction: number;
  holdRightClick: boolean;
  rightClickHoldMs: number;
  rightClickSlop: number;
  customKeyboard: boolean;
  volumeKeysControlOutput: boolean;
}

export const DEFAULT_SETTINGS: TrackpadSettings = {
  terminalScrollback: false,
  terminalFont: "iosevka",
  autocorrect: false,
  adaptiveKeys: false,
  debugKeySizes: false,
  previewNavigation: true,
  spaceCursorSensitivity: 2,
  interactivePreview: false,
  mirrorFocusedText: false,
  keyboardLayout: "en-GB",
  keyboardTheme: "dark",
  tabKey: false,
  emojiKey: false,
  periodKey: false,
  commaKey: false,
  swipeTyping: true,
  cursorSensitivity: 1.5,
  cursorFriction: 0.92,
  scrollSensitivity: 1.5,
  scrollFriction: 0.92,
  holdRightClick: true,
  rightClickHoldMs: 500,
  rightClickSlop: 6,
  customKeyboard: true,
  volumeKeysControlOutput: true,
  suggestionsAboveControls: true,
  numberRow: true,
  tallerKeys: true,
  symbolHints: true,
  doubleSpacePeriod: true,
  haptics: true,
  longPressDelayMs: 250,
  spaceHoldDelayMs: 320,
  backspaceRepeatDelayMs: 430,
  backspaceRepeatIntervalMs: 65,
  shiftDoubleTapMs: 350,
};

export const KEYBOARD_TIMINGS = [
  { key: "longPressDelayMs", label: "Symbol hold", min: 150, max: 1000, step: 25 },
  { key: "spaceHoldDelayMs", label: "Space cursor hold", min: 150, max: 1000, step: 25 },
  { key: "backspaceRepeatDelayMs", label: "Backspace repeat", min: 150, max: 1000, step: 25 },
  { key: "backspaceRepeatIntervalMs", label: "Repeat interval", min: 25, max: 250, step: 5 },
  { key: "shiftDoubleTapMs", label: "Modifier double-tap", min: 200, max: 800, step: 25 },
] as const;

export const TRACKPAD_GESTURES = [
  { key: "rightClickHoldMs", label: "Hold duration", min: 250, max: 1000, step: 50, unit: "ms" },
  { key: "rightClickSlop", label: "Movement tolerance", min: 2, max: 20, step: 1, unit: "px" },
] as const;

export const TRACKPAD_RANGES = [
  { key: "cursorSensitivity", label: "Cursor Sensitivity", min: 0.1, max: 5, step: 0.1, decimals: 1 },
  { key: "cursorFriction", label: "Cursor Friction", min: 0.8, max: 0.99, step: 0.01, decimals: 2 },
  { key: "scrollSensitivity", label: "Scroll Sensitivity", min: 0.1, max: 5, step: 0.1, decimals: 1 },
  { key: "scrollFriction", label: "Scroll Friction", min: 0.8, max: 0.99, step: 0.01, decimals: 2 },
] as const;

// Treat browser storage as untrusted input. Bad values must not crash rendering
// (e.g. toFixed on a string) or create runaway inertia/repeat timers.
export function parseSettings(value: unknown): TrackpadSettings {
  const settings = { ...DEFAULT_SETTINGS };
  if (!value || typeof value !== "object" || Array.isArray(value)) return settings;
  const saved = value as Record<string, unknown>;
  for (const key of ["terminalScrollback", "tabKey", "emojiKey", "interactivePreview", "autocorrect", "adaptiveKeys", "debugKeySizes", "previewNavigation", "suggestionsAboveControls", "mirrorFocusedText", "numberRow", "tallerKeys", "symbolHints", "doubleSpacePeriod", "haptics", "holdRightClick", "customKeyboard", "volumeKeysControlOutput", "periodKey", "commaKey", "swipeTyping"] as const) {
    if (typeof saved[key] === "boolean") settings[key] = saved[key];
  }
  for (const { key, min, max } of [...KEYBOARD_TIMINGS, ...TRACKPAD_RANGES, ...TRACKPAD_GESTURES, { key: "spaceCursorSensitivity", min: 0.5, max: 5 } as const]) {
    const next = saved[key];
    if (typeof next === "number" && Number.isFinite(next)) {
      settings[key] = Math.min(max, Math.max(min, next));
    }
  }
  if (["en-GB", "en-US", "auto"].includes(String(saved.keyboardLayout)))
    settings.keyboardLayout = saved.keyboardLayout as TrackpadSettings["keyboardLayout"];
  if (["dark", "light", "system"].includes(String(saved.keyboardTheme)))
    settings.keyboardTheme = saved.keyboardTheme as TrackpadSettings["keyboardTheme"];
  if (typeof saved.terminalFont === "string" && Object.hasOwn(TERMINAL_FONTS, saved.terminalFont))
    settings.terminalFont = saved.terminalFont as KeyboardSettings["terminalFont"];
  return settings;
}
