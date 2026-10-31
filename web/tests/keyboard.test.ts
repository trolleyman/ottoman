import { describe, expect, test } from "bun:test";
import { keyboardRows, resolveKeyboardLayout } from "../src/keyboardData";
import { createSwipeDecoder, type KeyPositions, resample } from "../src/swipeTyping";
import { decodeSwipe } from "../src/swipeDictionary";
import { EMOJI_GROUPS, withSkinTone } from "../src/emojiData";

const positions: KeyPositions = {};
["qwertyuiop", "asdfghjkl", "zxcvbnm"].forEach((row, y) => {
  Array.from(row).forEach((letter, x) => { positions[letter] = { x: x + [0, 0.45, 1.3][y], y }; });
});
const pathFor = (word: string) => Array.from(word).filter((letter, index) => letter !== word[index - 1]).map((letter) => positions[letter]);

describe("keyboard layout", () => {
  test("uses UK shifts and promotes the exact letter hints to symbol keys", () => {
    const { numberRow, letterRows, symbolRows } = keyboardRows("en-GB");
    expect(letterRows[0][0]).toEqual({ key: "q", alt: "`" });
    expect(letterRows[2][1]).toEqual({ key: "x", alt: "|" });
    expect(numberRow[1].alt).toBe('"');
    expect(numberRow[2].alt).toBe("£");
    expect(letterRows[0][2]).toEqual({ key: "e", alt: "£" });
    expect(symbolRows.map((row) => row.map((key) => key.key))).toEqual(letterRows.map((row) => row.map((key) => key.alt!)));
    expect(symbolRows[1][0].key).toBe("-");
    expect(symbolRows[2].slice(-2).map((key) => key.key)).toEqual(["?", "'"]);
  });

  test("only infers locale when explicitly selected", () => {
    expect(resolveKeyboardLayout("en-GB", "en-US")).toBe("en-GB");
    expect(resolveKeyboardLayout("auto", "en-US")).toBe("en-US");
    expect(resolveKeyboardLayout("auto", "en-GB")).toBe("en-GB");
    expect(keyboardRows("en-US").numberRow[1].alt).toBe("@");
  });
});

describe("local swipe typing", () => {
  test("recognises common word paths using the full ranked dictionary", () => {
    for (const word of ["hello", "world", "keyboard", "swipe", "british", "thanks", "good", "morning"])
      expect(decodeSwipe(resample(pathFor(word), 40), positions)).toContain(word);
  });

  test("recognises contractions along their letter-only swipe paths", () => {
    for (const [path, word] of [["dont", "don't"], ["isnt", "isn't"], ["cant", "can't"]])
      expect(decodeSwipe(resample(pathFor(path), 40), positions)).toContain(word);
  });

  test("preserves turns and repeated letters in candidates", () => {
    const decode = createSwipeDecoder(["hell", "hello", "help", "hero", "world", "word"]);
    expect(decode(pathFor("hello"), positions)[0]).toBe("hello");
    expect(decode(pathFor("world"), positions)[0]).toBe("world");
    expect(decode([], positions)).toEqual([]);
  });

  test("resampling ignores dwell time and duplicate points", () => {
    const path = pathFor("swipe");
    const paused = path.flatMap((point) => [point, point, point]);
    expect(resample(paused)).toEqual(resample(path));
  });
});

test("emoji covers all major sections and supports skin tones", () => {
  expect(EMOJI_GROUPS).toHaveLength(9);
  expect(EMOJI_GROUPS.flatMap((group) => group.emojis).length).toBeGreaterThan(1900);
  expect(withSkinTone("👋", "🏽", true)).toBe("👋🏽");
  expect(withSkinTone("☝️", "🏽", true)).toBe("☝🏽");
  expect(withSkinTone("❤️", "🏽", false)).toBe("❤️");
});
