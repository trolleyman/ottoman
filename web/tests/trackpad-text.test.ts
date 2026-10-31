import { describe, expect, test } from "bun:test";
import { characterCount, diffText } from "../src/trackpadText";

describe("mobile trackpad text reconciliation", () => {
  test("appends ordinary and symbol-keyboard text", () => {
    expect(diffText("hello", "hello?[]")).toEqual({
      start: 5,
      removed: "",
      inserted: "?[]",
    });
  });

  test("mirrors iOS double-space punctuation as a replacement", () => {
    expect(diffText("Hello  ", "Hello. ")).toEqual({
      start: 5,
      removed: " ",
      inserted: ".",
    });
  });

  test("mirrors an autocorrected word without retyping the suffix", () => {
    expect(diffText("I dont know", "I don't know")).toEqual({
      start: 5,
      removed: "",
      inserted: "'",
    });
  });

  test("reports held-backspace deletion", () => {
    expect(diffText("testing", "test")).toEqual({
      start: 4,
      removed: "ing",
      inserted: "",
    });
  });

  test("counts a composed character as one remote key", () => {
    expect(characterCount("a👍b")).toBe(3);
  });

  test("replaces emoji sharing a surrogate without emitting half a character", () => {
    expect(diffText("a😀b", "a😎b")).toEqual({ start: 1, removed: "😀", inserted: "😎" });
    expect(diffText("a😀b", "a🨀b")).toEqual({ start: 1, removed: "😀", inserted: "🨀" });
  });

  test("keeps textarea offsets after non-BMP characters", () => {
    expect(diffText("👍hello", "👍hallo")).toEqual({ start: 3, removed: "e", inserted: "a" });
    expect(diffText("👍", "👍!")).toEqual({ start: 2, removed: "", inserted: "!" });
  });

});
