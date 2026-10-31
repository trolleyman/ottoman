import { afterAll, beforeAll, afterEach, describe, expect, spyOn, test } from "bun:test";
import { DEFAULT_SETTINGS, parseSettings } from "../src/trackpadSettings";
import { loadRecentEmoji, loadSettings, savePreference } from "../src/preferences";

const originalStorage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
const values = new Map<string, string>();
beforeAll(() => {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      clear: () => values.clear(),
    },
  });
});
afterAll(() => {
  if (originalStorage) Object.defineProperty(globalThis, "localStorage", originalStorage);
  else Reflect.deleteProperty(globalThis, "localStorage");
});

afterEach(() => {
  localStorage.clear();
});

describe("trackpad preferences", () => {
  test("defaults missing fields while keeping valid preferences", () => {
    expect(parseSettings({ numberRow: false, cursorSensitivity: 2 })).toEqual({
      ...DEFAULT_SETTINGS, numberRow: false, cursorSensitivity: 2,
    });
  });

  test("keeps native mirroring opt-in and restores only a boolean preference", () => {
    expect(loadSettings().mirrorFocusedText).toBe(false);
    savePreference("ottoman-trackpad-settings", { mirrorFocusedText: true });
    expect(loadSettings().mirrorFocusedText).toBe(true);
    expect(parseSettings({ mirrorFocusedText: "true" }).mirrorFocusedText).toBe(false);
  });

  test("rejects invalid types and non-finite values", () => {
    for (const value of [null, [], "settings", 5]) expect(parseSettings(value)).toEqual(DEFAULT_SETTINGS);
    expect(parseSettings({ cursorSensitivity: "fast", cursorFriction: Infinity, longPressDelayMs: NaN, haptics: "false" })).toEqual(DEFAULT_SETTINGS);
  });

  test("clamps stored values to the same bounds as the controls", () => {
    const settings = parseSettings({ cursorFriction: 1, cursorSensitivity: -1, backspaceRepeatIntervalMs: 0, longPressDelayMs: 100000 });
    expect(settings.cursorFriction).toBe(0.99);
    expect(settings.cursorSensitivity).toBe(0.1);
    expect(settings.backspaceRepeatIntervalMs).toBe(25);
    expect(settings.longPressDelayMs).toBe(1000);
  });

  test("keeps explicit layout, appearance, swipe and period preferences", () => {
    expect(parseSettings({ keyboardLayout: "en-US", keyboardTheme: "light", periodKey: false, swipeTyping: false })).toEqual({
      ...DEFAULT_SETTINGS, keyboardLayout: "en-US", keyboardTheme: "light", periodKey: false, swipeTyping: false,
    });
    expect(parseSettings({ keyboardLayout: "invalid", keyboardTheme: "bright", periodKey: "no" })).toEqual(DEFAULT_SETTINGS);
  });

  test("recovers from malformed JSON and non-array emoji storage", () => {
    localStorage.setItem("ottoman-trackpad-settings", "{");
    localStorage.setItem("ottoman-recent-emoji", "null");
    expect(loadSettings()).toEqual(DEFAULT_SETTINGS);
    expect(loadRecentEmoji()).toContain("😂");
  });

  test("deduplicates recent emoji and discards invalid entries", () => {
    savePreference("ottoman-recent-emoji", ["😂", null, "😂", {}, "not an emoji", "🔥"]);
    expect(loadRecentEmoji()).toEqual(["😂", "🔥"]);
  });

  test("unavailable storage does not break settings or typing", () => {
    const read = spyOn(localStorage, "getItem").mockImplementation(() => { throw new Error("disabled"); });
    const write = spyOn(localStorage, "setItem").mockImplementation(() => { throw new Error("quota"); });
    try {
      expect(loadSettings()).toEqual(DEFAULT_SETTINGS);
      expect(() => savePreference("ottoman-recent-emoji", ["🔥"])).not.toThrow();
    } finally {
      read.mockRestore();
      write.mockRestore();
    }
  });
});
