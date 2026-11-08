import { expect, test } from "bun:test";
import { Modifier } from "../src/api";
import { createAutocorrect, KeyboardModifiers, nextKeyWeights, spaceEdit } from "../src/keyboardEditing";
import { parseSettings } from "../src/trackpadSettings";

test("all modifiers support one-shot, double-tap lock, hold and cancellation", () => {
  for (const key of [Modifier.SHIFT, Modifier.CTRL, Modifier.ALT, Modifier.META]) {
    const state = new KeyboardModifiers();
    state.press(key); state.release(key, 100, 350);
    expect(state.active).toEqual([key]);
    state.consume(); expect(state.active).toEqual([]);
    state.press(key); state.release(key, 1000, 350);
    state.press(key); state.release(key, 1200, 350);
    expect(state.states.get(key)).toBe("locked");
    state.consume(); expect(state.active).toEqual([key]);
    state.press(key); state.release(key, 1800, 350);
    expect(state.active).toEqual([]);
    state.press(key); state.release(key, 1850, 350);
    expect(state.states.get(key)).toBe("once");
    state.consume();
    state.press(key); state.consume(); state.consume();
    expect(state.active).toEqual([key]);
    state.release(key, 2500, 350); expect(state.active).toEqual([]);
    state.press(key); state.release(key, 3000, 350, true);
    expect(state.active).toEqual([]);
  }
});

test("double-space works after navigation and only suppresses an explicit undo", () => {
  expect(spaceEdit("sentence ", " ", true, false, false)).toEqual({ remove: 1, text: ". " });
  expect(spaceEdit("sentence ", " ", true, true, false)).toEqual({ remove: 0, text: " " });
  expect(spaceEdit("sentence. ", " ", true, false, false).remove).toBe(0);
});

test("punctuation removes only an accepted word's automatic separator", () => {
  expect(spaceEdit("hello ", ",", true, false, true)).toEqual({ remove: 1, text: "," });
  expect(spaceEdit("hello ", ",", true, false, false)).toEqual({ remove: 0, text: "," });
  expect(spaceEdit("hello ", "w", true, false, true)).toEqual({ remove: 0, text: "w" });
});

test("autocorrect preserves valid words and case, handles transpositions and keynoard", () => {
  const correct = createAutocorrect(["the", "keyboard", "hello", "world"]);
  expect(correct("keynoard")).toBe("keyboard");
  expect(correct("Keynoard")).toBe("Keyboard");
  expect(correct("KEYNOARD")).toBe("KEYBOARD");
  expect(correct("teh")).toBe("the");
  expect(correct("world")).toBe("world");
  expect(correct("xyzzy")).toBe("xyzzy");
});

test("predicted next letters widen within bounds and options persist", () => {
  const weights = nextKeyWeights("hel", ["hello", "help", "helping"]);
  expect(weights.l).toBeGreaterThan(1);
  expect(weights.p).toBeLessThanOrEqual(1.8);
  expect(nextKeyWeights("", ["hello"])).toEqual({});
  expect(parseSettings({}).autocorrect).toBe(false);
  const saved = parseSettings({ autocorrect: true, adaptiveKeys: true, debugKeySizes: true, previewNavigation: false, spaceCursorSensitivity: 100 });
  expect(saved.autocorrect && saved.adaptiveKeys && saved.debugKeySizes).toBe(true);
  expect(saved.previewNavigation).toBe(false);
  expect(saved.spaceCursorSensitivity).toBe(5);
});
