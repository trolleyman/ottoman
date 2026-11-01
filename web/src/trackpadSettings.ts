export interface KeyboardSettings {
  mirrorFocusedText: boolean;
  keyboardLayout: "en-GB" | "en-US" | "auto";
  keyboardTheme: "dark" | "light" | "system";
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
  clickAndDrag: boolean;
  customKeyboard: boolean;
}

export const DEFAULT_SETTINGS: TrackpadSettings = {
  mirrorFocusedText: false,
  keyboardLayout: "en-GB",
  keyboardTheme: "dark",
  periodKey: true,
  commaKey: true,
  swipeTyping: true,
  cursorSensitivity: 1.5,
  cursorFriction: 0.92,
  scrollSensitivity: 1.5,
  scrollFriction: 0.92,
  clickAndDrag: false,
  customKeyboard: true,
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
  { key: "shiftDoubleTapMs", label: "Shift double-tap", min: 200, max: 800, step: 25 },
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
  for (const key of ["suggestionsAboveControls", "mirrorFocusedText", "numberRow", "tallerKeys", "symbolHints", "doubleSpacePeriod", "haptics", "clickAndDrag", "customKeyboard", "periodKey", "commaKey", "swipeTyping"] as const) {
    if (typeof saved[key] === "boolean") settings[key] = saved[key];
  }
  for (const { key, min, max } of [...KEYBOARD_TIMINGS, ...TRACKPAD_RANGES]) {
    const next = saved[key];
    if (typeof next === "number" && Number.isFinite(next)) {
      settings[key] = Math.min(max, Math.max(min, next));
    }
  }
  if (["en-GB", "en-US", "auto"].includes(String(saved.keyboardLayout)))
    settings.keyboardLayout = saved.keyboardLayout as TrackpadSettings["keyboardLayout"];
  if (["dark", "light", "system"].includes(String(saved.keyboardTheme)))
    settings.keyboardTheme = saved.keyboardTheme as TrackpadSettings["keyboardTheme"];
  return settings;
}
